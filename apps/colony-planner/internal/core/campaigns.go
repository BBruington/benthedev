package core

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/BBruington/colony-planner/internal/store"
)

// Account-level operations (docs/design/multi-campaign.md §5): they take a Caller, never a Viewer, and
// read only the caller's own memberships, their own requests and the directory data of an invite.

// AccountMe is GET /api/me: who is signed in, before any campaign.
type AccountMe struct {
	AccountID       uuid.UUID `json:"accountId" format:"uuid"`
	DisplayName     string    `json:"displayName"`
	DiscordUsername string    `json:"discordUsername"`
	AvatarURL       *string   `json:"avatarUrl"`
	// IsSiteOwner is about the caller themselves; it shows the Site admin entry.
	IsSiteOwner  bool `json:"isSiteOwner"`
	ViewAsPlayer bool `json:"viewAsPlayer"`
}

// MyCampaign is one card on the picker: a campaign the caller has a membership in.
type MyCampaign struct {
	Campaign      CampaignSummary `json:"campaign"`
	MemberID      uuid.UUID       `json:"memberId" format:"uuid"`
	Role          CampaignRole    `json:"role"`
	Status        MemberStatus    `json:"status"`
	CharacterName *string         `json:"characterName"`
}

type MyCampaigns struct {
	Campaigns []MyCampaign `json:"campaigns"`
}

// CampaignRequestInput asks the site owner for a campaign.
type CampaignRequestInput struct {
	CampaignName string `json:"campaignName" doc:"1 to 80 characters after trimming"`
	Slug         string `json:"slug" doc:"The address /c/<slug>: 3 to 40 of a-z, 0-9 and single dashes"`
	Note         string `json:"note" doc:"To the site owner; at most 500 characters"`
}

// CampaignRequestView is the caller's own request, or one on the owner's list.
type CampaignRequestView struct {
	ID           uuid.UUID             `json:"id" format:"uuid"`
	CampaignName string                `json:"campaignName"`
	Slug         string                `json:"slug"`
	Note         string                `json:"note"`
	Status       CampaignRequestStatus `json:"status"`
	CreatedAt    time.Time             `json:"createdAt"`
	DecidedAt    *time.Time            `json:"decidedAt"`
	// Campaign is set once the request is approved and the campaign still exists, for "Open it".
	Campaign *CampaignSummary `json:"campaign" doc:"null until approved"`
}

// MyCampaignRequest is /request: the caller's newest request, or null.
type MyCampaignRequest struct {
	Request *CampaignRequestView `json:"request" doc:"null when the caller has never asked"`
}

// InviteLink is the DM's invite card. The code appears in no other DTO.
type InviteLink struct {
	InviteCode string `json:"inviteCode"`
}

// InviteInfo is /join/{inviteCode}: who runs the campaign and where the caller stands there.
type InviteInfo struct {
	CampaignName   string         `json:"campaignName"`
	DmDisplayName  string         `json:"dmDisplayName"`
	CampaignStatus CampaignStatus `json:"campaignStatus"`
	// Membership is the caller's own, or null when they have none there yet.
	Membership *MyCampaign `json:"membership" doc:"null when the caller has not asked to join"`
}

// DeleteCampaignInput confirms a deletion by typing the campaign's slug.
type DeleteCampaignInput struct {
	ConfirmSlug string `json:"confirmSlug" doc:"The campaign's address, typed to confirm"`
}

var (
	slugPattern         = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	inviteCodePattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{22,64}$`)
	ErrInviteNotWorking = &Error{Kind: KindNotFound, Message: "This invite link doesn't work any more. Ask your DM for a new one."}
	ErrJoinsFull        = &Error{Kind: KindConflict, Message: "Not accepting new players right now."}
)

func (service *Service) AccountMe(requestContext context.Context, caller Caller) (AccountMe, error) {
	if err := requireSignedIn(caller); err != nil {
		return AccountMe{}, err
	}
	account, err := service.queries.GetAccount(requestContext, caller.AccountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountMe{}, service.sessionEnded()
	}
	if err != nil {
		return AccountMe{}, err
	}
	return AccountMe{
		AccountID:       account.ID,
		DisplayName:     account.DisplayName,
		DiscordUsername: account.DiscordUsername,
		AvatarURL:       account.AvatarUrl,
		IsSiteOwner:     caller.IsSiteOwner,
		ViewAsPlayer:    caller.ViewAsPlayer,
	}, nil
}

func toMyCampaign(campaign store.Campaign, membership store.CampaignMember) MyCampaign {
	return MyCampaign{
		Campaign:      toCampaignSummary(campaign),
		MemberID:      membership.ID,
		Role:          membership.Role,
		Status:        membership.Status,
		CharacterName: membership.CharacterName,
	}
}

// MyCampaigns is the picker: every campaign the caller has a membership in, whatever its status.
func (service *Service) MyCampaigns(requestContext context.Context, caller Caller) (MyCampaigns, error) {
	if err := requireSignedIn(caller); err != nil {
		return MyCampaigns{}, err
	}
	rows, err := service.queries.ListMembershipsForAccount(requestContext, caller.AccountID)
	if err != nil {
		return MyCampaigns{}, err
	}
	campaigns := make([]MyCampaign, 0, len(rows))
	for _, row := range rows {
		campaigns = append(campaigns, toMyCampaign(row.Campaign, row.CampaignMember))
	}
	return MyCampaigns{Campaigns: campaigns}, nil
}

func newInviteCode() (string, error) {
	codeBytes := make([]byte, 16)
	if _, err := rand.Read(codeBytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(codeBytes), nil
}

// checkCampaignRequest cleans a request's fields; the messages are the person's.
func checkCampaignRequest(input CampaignRequestInput) (CampaignRequestInput, []FieldError) {
	var fieldErrors []FieldError
	campaignName, fieldError := cleanText("campaignName", input.CampaignName, campaignNameMaxLength)
	switch {
	case fieldError != nil:
		fieldErrors = append(fieldErrors, *fieldError)
	case campaignName == "":
		fieldErrors = append(fieldErrors, FieldError{Field: "campaignName", Message: "Give the campaign a name."})
	}
	slug, fieldError := checkSlug("slug", input.Slug)
	if fieldError != nil {
		fieldErrors = append(fieldErrors, *fieldError)
	}
	note, fieldError := cleanMultilineText("note", input.Note, requestNoteMaxLength)
	if fieldError != nil {
		fieldErrors = append(fieldErrors, *fieldError)
	}
	return CampaignRequestInput{CampaignName: campaignName, Slug: slug, Note: note}, fieldErrors
}

func checkSlug(field string, value string) (string, *FieldError) {
	slug, fieldError := cleanText(field, value, slugMaxLength)
	if fieldError != nil {
		return "", fieldError
	}
	if len(slug) < slugMinLength || !slugPattern.MatchString(slug) {
		return "", &FieldError{Field: field, Message: "Use 3 to 40 of a-z, 0-9 and single dashes, such as sky-isles."}
	}
	return slug, nil
}

// RequestCampaign asks the site owner for a campaign: one open request per account, and the slug must
// be free among campaigns and open requests. The site owner's own request is approved at once.
func (service *Service) RequestCampaign(requestContext context.Context, caller Caller, input CampaignRequestInput) (CampaignRequestView, error) {
	if err := requireSignedIn(caller); err != nil {
		return CampaignRequestView{}, err
	}
	cleaned, fieldErrors := checkCampaignRequest(input)
	if len(fieldErrors) > 0 {
		return CampaignRequestView{}, Invalid("The request needs fixing.", fieldErrors...)
	}
	var request store.CampaignRequest
	var account store.Account
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		var err error
		account, err = queries.GetAccount(requestContext, caller.AccountID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionEnded
		}
		if err != nil {
			return err
		}
		open, err := queries.CountOpenCampaignRequestsForAccount(requestContext, caller.AccountID)
		if err != nil {
			return err
		}
		if open >= OpenCampaignRequestsPerAccount {
			return Conflict("You already asked. Withdraw it to change it.")
		}
		if err := slugFree(requestContext, queries, cleaned.Slug, uuid.Nil); err != nil {
			return err
		}
		request, err = queries.CreateCampaignRequest(requestContext, store.CreateCampaignRequestParams{
			AccountID:    caller.AccountID,
			CampaignName: cleaned.CampaignName,
			Slug:         cleaned.Slug,
			Note:         cleaned.Note,
		})
		return uniqueSlugConflict(err)
	})
	if err != nil {
		return CampaignRequestView{}, err
	}
	if caller.IsSiteOwner {
		if _, err := service.ApproveCampaignRequest(requestContext, caller, request.ID, ApproveCampaignRequestInput{}); err != nil {
			return CampaignRequestView{}, err
		}
		return service.campaignRequestView(requestContext, request.ID, caller.AccountID)
	}
	service.notifier.CampaignRequested(context.WithoutCancel(requestContext), RequestedCampaign{
		CampaignRequestID: request.ID,
		CampaignName:      request.CampaignName,
		DisplayName:       account.DisplayName,
		DiscordUsername:   account.DiscordUsername,
	})
	return toCampaignRequestView(request, nil), nil
}

func slugFree(requestContext context.Context, queries *store.Queries, slug string, exceptRequestID uuid.UUID) error {
	taken, err := queries.SlugTaken(requestContext, store.SlugTakenParams{Slug: slug, ExceptRequestID: exceptRequestID})
	if err != nil {
		return err
	}
	if taken != nil && *taken {
		return slugTakenError()
	}
	return nil
}

func slugTakenError() error {
	return Invalid("That address is taken.", FieldError{Field: "slug", Message: "That address is taken. Try another."})
}

// uniqueSlugConflict turns a unique violation that slipped past slugFree (two requests at once) into
// the person's message.
func uniqueSlugConflict(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		if postgresError.ConstraintName == "campaign_request_one_open" {
			return Conflict("You already asked. Withdraw it to change it.")
		}
		return slugTakenError()
	}
	return err
}

func toCampaignRequestView(request store.CampaignRequest, campaign *store.Campaign) CampaignRequestView {
	view := CampaignRequestView{
		ID:           request.ID,
		CampaignName: request.CampaignName,
		Slug:         request.Slug,
		Note:         request.Note,
		Status:       request.Status,
		CreatedAt:    request.CreatedAt.UTC(),
		DecidedAt:    utcPointer(request.DecidedAt),
	}
	if campaign != nil {
		summary := toCampaignSummary(*campaign)
		view.Campaign = &summary
	}
	return view
}

func (service *Service) campaignRequestView(requestContext context.Context, requestID uuid.UUID, accountID uuid.UUID) (CampaignRequestView, error) {
	mine, err := service.MyCampaignRequest(requestContext, Caller{AccountID: accountID})
	if err != nil {
		return CampaignRequestView{}, err
	}
	if mine.Request == nil || mine.Request.ID != requestID {
		return CampaignRequestView{}, NotFound("No such request.")
	}
	return *mine.Request, nil
}

// MyCampaignRequest is the caller's newest request, whatever its status: "Waiting for the site
// owner", "The site owner said no this time" or "<name> is ready".
func (service *Service) MyCampaignRequest(requestContext context.Context, caller Caller) (MyCampaignRequest, error) {
	if err := requireSignedIn(caller); err != nil {
		return MyCampaignRequest{}, err
	}
	request, err := service.queries.LatestCampaignRequestForAccount(requestContext, caller.AccountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return MyCampaignRequest{}, nil
	}
	if err != nil {
		return MyCampaignRequest{}, err
	}
	var campaign *store.Campaign
	if request.CampaignID != nil {
		row, err := service.queries.GetCampaign(requestContext, *request.CampaignID)
		switch {
		case err == nil:
			campaign = &row
		case !errors.Is(err, pgx.ErrNoRows):
			return MyCampaignRequest{}, err
		}
	}
	view := toCampaignRequestView(request, campaign)
	return MyCampaignRequest{Request: &view}, nil
}

// WithdrawCampaignRequest takes back the caller's own open request (to change it, withdraw and ask
// again).
func (service *Service) WithdrawCampaignRequest(requestContext context.Context, caller Caller, campaignRequestID uuid.UUID) (CampaignRequestView, error) {
	if err := requireSignedIn(caller); err != nil {
		return CampaignRequestView{}, err
	}
	var request store.CampaignRequest
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		current, err := queries.GetCampaignRequestForUpdate(requestContext, campaignRequestID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && current.AccountID != caller.AccountID) {
			return NotFound("No such request.")
		}
		if err != nil {
			return err
		}
		if current.Status != store.CampaignRequestStatusPENDING {
			return Conflict("The site owner has already decided.")
		}
		request, err = queries.DecideCampaignRequest(requestContext, store.DecideCampaignRequestParams{
			Status: store.CampaignRequestStatusWITHDRAWN,
			Slug:   current.Slug,
			ID:     current.ID,
		})
		return err
	})
	if err != nil {
		return CampaignRequestView{}, err
	}
	return toCampaignRequestView(request, nil), nil
}

// Invite is the DM's invite card: the code is served here and by ResetInviteLink only.
func (service *Service) Invite(requestContext context.Context, viewer Viewer) (InviteLink, error) {
	if err := requireDm(viewer); err != nil {
		return InviteLink{}, err
	}
	campaign, err := service.queries.GetCampaign(requestContext, viewer.campaignID)
	if err != nil {
		return InviteLink{}, campaignGone(err)
	}
	return InviteLink{InviteCode: campaign.InviteCode}, nil
}

// ResetInviteLink kills the old link at once; people already waiting stay waiting.
func (service *Service) ResetInviteLink(requestContext context.Context, viewer Viewer) (InviteLink, error) {
	if err := requireDm(viewer); err != nil {
		return InviteLink{}, err
	}
	inviteCode, err := newInviteCode()
	if err != nil {
		return InviteLink{}, err
	}
	campaign, err := service.queries.SetCampaignInviteCode(requestContext, store.SetCampaignInviteCodeParams{CampaignID: viewer.campaignID, InviteCode: inviteCode})
	if err != nil {
		return InviteLink{}, campaignGone(err)
	}
	return InviteLink{InviteCode: campaign.InviteCode}, nil
}

func (service *Service) campaignByInvite(requestContext context.Context, queries *store.Queries, inviteCode string) (store.Campaign, error) {
	if !inviteCodePattern.MatchString(inviteCode) {
		return store.Campaign{}, ErrInviteNotWorking
	}
	campaign, err := queries.CampaignByInviteCode(requestContext, inviteCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Campaign{}, ErrInviteNotWorking
	}
	return campaign, err
}

// InviteInfo is what /join/{inviteCode} shows: the campaign's name, its DM, whether it is paused, and
// the caller's own membership there if they have one.
func (service *Service) InviteInfo(requestContext context.Context, caller Caller, inviteCode string) (InviteInfo, error) {
	if err := requireSignedIn(caller); err != nil {
		return InviteInfo{}, err
	}
	campaign, err := service.campaignByInvite(requestContext, service.queries, inviteCode)
	if err != nil {
		return InviteInfo{}, err
	}
	dm, err := service.queries.GetCampaignDm(requestContext, campaign.ID)
	if err != nil {
		return InviteInfo{}, campaignGone(err)
	}
	info := InviteInfo{CampaignName: campaign.Name, DmDisplayName: dm.Account.DisplayName, CampaignStatus: campaign.Status}
	membership, err := service.queries.GetMembershipForAccount(requestContext, store.GetMembershipForAccountParams{CampaignID: campaign.ID, AccountID: caller.AccountID})
	switch {
	case err == nil:
		mine := toMyCampaign(membership.Campaign, membership.CampaignMember)
		info.Membership = &mine
	case !errors.Is(err, pgx.ErrNoRows):
		return InviteInfo{}, err
	}
	return info, nil
}

// JoinByInvite asks to join. Under a per-campaign lock: a suspended campaign refuses; an existing
// membership comes back unchanged and pings nobody (a REMOVED one stays REMOVED); past
// PendingMemberLimit waiting it refuses; otherwise a new PENDING membership, and Beny Bot pings the DM.
func (service *Service) JoinByInvite(requestContext context.Context, caller Caller, inviteCode string) (MyCampaign, error) {
	if err := requireSignedIn(caller); err != nil {
		return MyCampaign{}, err
	}
	var joined MyCampaign
	var account store.Account
	created := false
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		campaign, err := service.campaignByInvite(requestContext, queries, inviteCode)
		if err != nil {
			return err
		}
		if err := queries.LockCampaignJoins(requestContext, campaign.ID); err != nil {
			return err
		}
		// Re-read under the lock: a suspension or a reset may have committed meanwhile.
		campaign, err = service.campaignByInvite(requestContext, queries, inviteCode)
		if err != nil {
			return err
		}
		if campaign.Status != store.CampaignStatusACTIVE {
			return Conflict(ErrCampaignSuspended.Message)
		}
		existing, err := queries.GetCampaignMemberByAccountForUpdate(requestContext, store.GetCampaignMemberByAccountForUpdateParams{CampaignID: campaign.ID, AccountID: caller.AccountID})
		if err == nil {
			joined = toMyCampaign(campaign, existing)
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		pending, err := queries.CountPendingCampaignMembers(requestContext, campaign.ID)
		if err != nil {
			return err
		}
		if pending >= PendingMemberLimit {
			return ErrJoinsFull
		}
		account, err = queries.GetAccount(requestContext, caller.AccountID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionEnded
		}
		if err != nil {
			return err
		}
		membership, err := queries.CreateCampaignMember(requestContext, store.CreateCampaignMemberParams{
			CampaignID: campaign.ID,
			AccountID:  caller.AccountID,
			Role:       store.CampaignRolePLAYER,
			Status:     store.MemberStatusPENDING,
		})
		if err != nil {
			return err
		}
		joined = toMyCampaign(campaign, membership)
		created = true
		return nil
	})
	if err != nil {
		return MyCampaign{}, err
	}
	if created {
		service.broker.Publish(joined.Campaign.ID, TopicMembers)
		// The request may end before Discord answers; the ping is best-effort and must not be cut short.
		service.notifier.MemberWaiting(context.WithoutCancel(requestContext), WaitingMember{
			CampaignID:      joined.Campaign.ID,
			CampaignSlug:    joined.Campaign.Slug,
			MemberID:        joined.MemberID,
			DisplayName:     account.DisplayName,
			DiscordUsername: account.DiscordUsername,
		})
	}
	return joined, nil
}

// DeleteOwnCampaign is the DM's "Delete this campaign" (decision C3): at any time, after typing the
// slug. It cascades exactly like the owner's delete.
func (service *Service) DeleteOwnCampaign(requestContext context.Context, viewer Viewer, input DeleteCampaignInput) error {
	if err := requireDm(viewer); err != nil {
		return err
	}
	return service.deleteCampaign(requestContext, viewer.campaignID, input.ConfirmSlug, false)
}

// deleteCampaign checks the typed slug (and, for the owner, the suspension) under the campaign's row
// lock, deletes it with everything it owns, and closes its streams, link code and resume ring.
func (service *Service) deleteCampaign(requestContext context.Context, campaignID uuid.UUID, confirmSlug string, requireSuspended bool) error {
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		campaign, err := queries.GetCampaignForUpdate(requestContext, campaignID)
		if err != nil {
			return campaignGone(err)
		}
		if requireSuspended && campaign.Status != store.CampaignStatusSUSPENDED {
			return Conflict("Suspend the campaign before deleting it.")
		}
		if confirmSlug != campaign.Slug {
			message := "Type " + campaign.Slug + " to confirm."
			return Invalid(message, FieldError{Field: "confirmSlug", Message: message})
		}
		_, err = queries.DeleteCampaign(requestContext, campaignID)
		return err
	})
	if err != nil {
		return err
	}
	service.linkCodes.drop(campaignID)
	service.broker.CloseCampaign(campaignID)
	service.logger.InfoContext(requestContext, "campaign deleted", "campaignId", campaignID.String())
	return nil
}
