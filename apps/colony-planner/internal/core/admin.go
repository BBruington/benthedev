package core

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// The site owner manages campaigns but never reads them (docs/design/multi-campaign.md §4.3). Every
// function here takes a Caller, checks IsSiteOwner, and reads only directory tables: no colony,
// timeline, settings or character names. Being owner confers nothing inside a campaign.

// AdminCampaign is one row of /admin → Campaigns.
type AdminCampaign struct {
	ID                uuid.UUID      `json:"id" format:"uuid"`
	Slug              string         `json:"slug"`
	Name              string         `json:"name"`
	Status            CampaignStatus `json:"status"`
	DmDisplayName     string         `json:"dmDisplayName"`
	DmDiscordUsername string         `json:"dmDiscordUsername"`
	PlayerCount       int64          `json:"playerCount"`
	PendingCount      int64          `json:"pendingCount"`
	DiscordLinked     bool           `json:"discordLinked"`
	CreatedAt         time.Time      `json:"createdAt"`
}

type AdminCampaignList struct {
	Campaigns []AdminCampaign `json:"campaigns"`
}

// AdminCampaignRequest is one open request on /admin → Requests.
type AdminCampaignRequest struct {
	Request         CampaignRequestView `json:"request"`
	DisplayName     string              `json:"displayName"`
	DiscordUsername string              `json:"discordUsername"`
	AvatarURL       *string             `json:"avatarUrl"`
}

type AdminCampaignRequestList struct {
	Requests []AdminCampaignRequest `json:"requests"`
}

// ApproveCampaignRequestInput lets the owner adjust the address at approval; it is fixed afterwards.
type ApproveCampaignRequestInput struct {
	Slug *string `json:"slug" doc:"null keeps the requested address"`
}

func (service *Service) ListCampaignRequests(requestContext context.Context, caller Caller) (AdminCampaignRequestList, error) {
	if err := requireSiteOwner(caller); err != nil {
		return AdminCampaignRequestList{}, err
	}
	rows, err := service.queries.ListOpenCampaignRequests(requestContext)
	if err != nil {
		return AdminCampaignRequestList{}, err
	}
	requests := make([]AdminCampaignRequest, 0, len(rows))
	for _, row := range rows {
		requests = append(requests, AdminCampaignRequest{
			Request:         toCampaignRequestView(row.CampaignRequest, nil),
			DisplayName:     row.Account.DisplayName,
			DiscordUsername: row.Account.DiscordUsername,
			AvatarURL:       row.Account.AvatarUrl,
		})
	}
	return AdminCampaignRequestList{Requests: requests}, nil
}

// ApproveCampaignRequest creates the campaign in one transaction: lock the request (it must be
// PENDING), re-check the slug, insert the campaign with a random invite code, its DM membership, its
// settings and colony (column defaults), its default resources and roles, and mark the request
// APPROVED. Only this (and the seed through it) ever inserts a DM.
func (service *Service) ApproveCampaignRequest(requestContext context.Context, caller Caller, campaignRequestID uuid.UUID, input ApproveCampaignRequestInput) (AdminCampaign, error) {
	if err := requireSiteOwner(caller); err != nil {
		return AdminCampaign{}, err
	}
	var campaignID uuid.UUID
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		request, err := queries.GetCampaignRequestForUpdate(requestContext, campaignRequestID)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound("No such request.")
		}
		if err != nil {
			return err
		}
		if request.Status != store.CampaignRequestStatusPENDING {
			return Conflict("That request was already decided or withdrawn.")
		}
		slug := request.Slug
		if input.Slug != nil {
			adjusted, fieldError := checkSlug("slug", *input.Slug)
			if fieldError != nil {
				return Invalid("Check the address.", *fieldError)
			}
			slug = adjusted
		}
		if err := slugFree(requestContext, queries, slug, request.ID); err != nil {
			return err
		}
		inviteCode, err := newInviteCode()
		if err != nil {
			return err
		}
		campaign, err := queries.CreateCampaign(requestContext, store.CreateCampaignParams{Slug: slug, Name: request.CampaignName, InviteCode: inviteCode})
		if err != nil {
			return uniqueSlugConflict(err)
		}
		campaignID = campaign.ID
		approvedAt := time.Now().UTC()
		if _, err := queries.CreateCampaignMember(requestContext, store.CreateCampaignMemberParams{
			CampaignID: campaign.ID,
			AccountID:  request.AccountID,
			Role:       store.CampaignRoleDM,
			Status:     store.MemberStatusACTIVE,
			ApprovedAt: &approvedAt,
		}); err != nil {
			return err
		}
		if err := queries.CreateSettings(requestContext, campaign.ID); err != nil {
			return err
		}
		if err := queries.CreateColony(requestContext, campaign.ID); err != nil {
			return err
		}
		if err := queries.CreateDefaultResources(requestContext, campaign.ID); err != nil {
			return err
		}
		if err := queries.CreateDefaultRoles(requestContext, campaign.ID); err != nil {
			return err
		}
		if err := createDefaultMarketOffers(requestContext, queries, campaign.ID); err != nil {
			return err
		}
		_, err = queries.DecideCampaignRequest(requestContext, store.DecideCampaignRequestParams{
			Status:     store.CampaignRequestStatusAPPROVED,
			Slug:       slug,
			CampaignID: &campaign.ID,
			ID:         request.ID,
		})
		return err
	})
	if err != nil {
		return AdminCampaign{}, err
	}
	service.logger.InfoContext(requestContext, "campaign approved", "campaignId", campaignID.String(), "campaignRequestId", campaignRequestID.String())
	return service.adminCampaign(requestContext, campaignID)
}

// DenyCampaignRequest says no; the requester may ask again.
func (service *Service) DenyCampaignRequest(requestContext context.Context, caller Caller, campaignRequestID uuid.UUID) (CampaignRequestView, error) {
	if err := requireSiteOwner(caller); err != nil {
		return CampaignRequestView{}, err
	}
	var decided store.CampaignRequest
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		request, err := queries.GetCampaignRequestForUpdate(requestContext, campaignRequestID)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound("No such request.")
		}
		if err != nil {
			return err
		}
		if request.Status != store.CampaignRequestStatusPENDING {
			return Conflict("That request was already decided or withdrawn.")
		}
		decided, err = queries.DecideCampaignRequest(requestContext, store.DecideCampaignRequestParams{
			Status: store.CampaignRequestStatusDENIED,
			Slug:   request.Slug,
			ID:     request.ID,
		})
		return err
	})
	if err != nil {
		return CampaignRequestView{}, err
	}
	return toCampaignRequestView(decided, nil), nil
}

func toAdminCampaign(row store.ListCampaignsForOwnerRow) AdminCampaign {
	return AdminCampaign{
		ID:                row.ID,
		Slug:              row.Slug,
		Name:              row.Name,
		Status:            row.Status,
		DmDisplayName:     row.DmDisplayName,
		DmDiscordUsername: row.DmDiscordUsername,
		PlayerCount:       row.PlayerCount,
		PendingCount:      row.PendingCount,
		DiscordLinked:     row.DiscordLinked,
		CreatedAt:         row.CreatedAt.UTC(),
	}
}

// ListCampaigns is /admin → Campaigns: directory columns and counts, no campaign content.
func (service *Service) ListCampaigns(requestContext context.Context, caller Caller) (AdminCampaignList, error) {
	if err := requireSiteOwner(caller); err != nil {
		return AdminCampaignList{}, err
	}
	rows, err := service.queries.ListCampaignsForOwner(requestContext)
	if err != nil {
		return AdminCampaignList{}, err
	}
	campaigns := make([]AdminCampaign, 0, len(rows))
	for _, row := range rows {
		campaigns = append(campaigns, toAdminCampaign(row))
	}
	return AdminCampaignList{Campaigns: campaigns}, nil
}

func (service *Service) adminCampaign(requestContext context.Context, campaignID uuid.UUID) (AdminCampaign, error) {
	rows, err := service.queries.ListCampaignsForOwner(requestContext)
	if err != nil {
		return AdminCampaign{}, err
	}
	for _, row := range rows {
		if row.ID == campaignID {
			return toAdminCampaign(row), nil
		}
	}
	return AdminCampaign{}, ErrNoSuchCampaign
}

// SuspendCampaign closes the campaign to everyone at once, data intact: campaign routes answer 403
// except the pending-level ones, its streams drop to `me`, its invite link refuses joins, its live link
// code is dropped, and Beny Bot answers its server with the paused line.
func (service *Service) SuspendCampaign(requestContext context.Context, caller Caller, campaignID uuid.UUID) (AdminCampaign, error) {
	return service.setCampaignStatus(requestContext, caller, campaignID, store.CampaignStatusSUSPENDED)
}

// UnsuspendCampaign restores everything; open tabs come back without a reload.
func (service *Service) UnsuspendCampaign(requestContext context.Context, caller Caller, campaignID uuid.UUID) (AdminCampaign, error) {
	return service.setCampaignStatus(requestContext, caller, campaignID, store.CampaignStatusACTIVE)
}

func (service *Service) setCampaignStatus(requestContext context.Context, caller Caller, campaignID uuid.UUID, status store.CampaignStatus) (AdminCampaign, error) {
	if err := requireSiteOwner(caller); err != nil {
		return AdminCampaign{}, err
	}
	changed := false
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		campaign, err := queries.GetCampaignForUpdate(requestContext, campaignID)
		if err != nil {
			return campaignGone(err)
		}
		if campaign.Status == status {
			return nil
		}
		if _, err := queries.SetCampaignStatus(requestContext, store.SetCampaignStatusParams{CampaignID: campaignID, Status: status}); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return AdminCampaign{}, err
	}
	if changed {
		active := status == store.CampaignStatusACTIVE
		if !active {
			service.linkCodes.drop(campaignID)
		}
		service.broker.campaignStatusChanged(campaignID, active)
		service.broker.publishToEveryStream(campaignID, TopicMe)
		if active {
			service.broker.Publish(campaignID, AllTopics()...)
		}
		service.logger.InfoContext(requestContext, "campaign status changed", "campaignId", campaignID.String(), "status", string(status))
	}
	return service.adminCampaign(requestContext, campaignID)
}

// DeleteCampaign is offered only for a suspended campaign, after typing its slug; everything the
// campaign owns cascades, and every account and its other campaigns stay.
func (service *Service) DeleteCampaign(requestContext context.Context, caller Caller, campaignID uuid.UUID, input DeleteCampaignInput) error {
	if err := requireSiteOwner(caller); err != nil {
		return err
	}
	return service.deleteCampaign(requestContext, campaignID, input.ConfirmSlug, true)
}
