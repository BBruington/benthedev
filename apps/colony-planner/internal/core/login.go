package core

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// LoginOptions tells /login which sign-in methods exist. It is public, so it carries nothing a
// visitor may not see; the account list appears only in devauth builds.
type LoginOptions struct {
	DiscordEnabled bool               `json:"discordEnabled"`
	DevSignIn      []DevSignInAccount `json:"devSignIn,omitempty" doc:"Present only in devauth builds"`
}

// DevSignInAccount is one person on the dev sign-in list, once, with their roles: "Ana · Thorin in
// Sample Colony · Brisa in Second Sample".
type DevSignInAccount struct {
	AccountID   uuid.UUID             `json:"accountId" format:"uuid"`
	DisplayName string                `json:"displayName"`
	IsSiteOwner bool                  `json:"isSiteOwner"`
	Memberships []DevSignInMembership `json:"memberships"`
	// AskingToRunCampaign is true while the account has an open campaign request.
	AskingToRunCampaign bool `json:"askingToRunCampaign"`
}

type DevSignInMembership struct {
	CampaignName  string       `json:"campaignName"`
	CampaignSlug  string       `json:"campaignSlug"`
	Role          CampaignRole `json:"role"`
	Status        MemberStatus `json:"status"`
	CharacterName *string      `json:"characterName"`
}

type LoginOptionsInput struct {
	DiscordEnabled bool
	// DevSignIn is true only in devauth builds; then every account is listed for one-click sign-in.
	DevSignIn bool
}

func (service *Service) LoginOptions(requestContext context.Context, input LoginOptionsInput) (LoginOptions, error) {
	options := LoginOptions{DiscordEnabled: input.DiscordEnabled}
	if !input.DevSignIn {
		return options, nil
	}
	accounts, err := service.DevSignInAccounts(requestContext)
	if err != nil {
		return LoginOptions{}, err
	}
	options.DevSignIn = accounts
	return options, nil
}

// DevSignInAccounts lists every account for dev sign-in, oldest first, with each membership. Only
// devauth builds call it; the list is small, so it reads each account's memberships in turn.
func (service *Service) DevSignInAccounts(requestContext context.Context) ([]DevSignInAccount, error) {
	rows, err := service.queries.ListAccounts(requestContext)
	if err != nil {
		return nil, err
	}
	accounts := make([]DevSignInAccount, 0, len(rows))
	for _, row := range rows {
		memberships, err := service.queries.ListMembershipsForAccount(requestContext, row.ID)
		if err != nil {
			return nil, err
		}
		account := DevSignInAccount{
			AccountID:   row.ID,
			DisplayName: row.DisplayName,
			IsSiteOwner: service.IsSiteOwner(row.DiscordUserID),
			Memberships: make([]DevSignInMembership, 0, len(memberships)),
		}
		for _, membership := range memberships {
			account.Memberships = append(account.Memberships, DevSignInMembership{
				CampaignName:  membership.Campaign.Name,
				CampaignSlug:  membership.Campaign.Slug,
				Role:          membership.CampaignMember.Role,
				Status:        membership.CampaignMember.Status,
				CharacterName: membership.CampaignMember.CharacterName,
			})
		}
		request, err := service.queries.LatestCampaignRequestForAccount(requestContext, row.ID)
		switch {
		case err == nil:
			account.AskingToRunCampaign = request.Status == store.CampaignRequestStatusPENDING
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

// AccountIDForDiscordUser finds an account by Discord ID (dev sign-in's `as=owner`); found is false
// when there is none.
func (service *Service) AccountIDForDiscordUser(requestContext context.Context, discordUserID string) (uuid.UUID, bool, error) {
	account, err := service.queries.GetAccountByDiscordUserID(requestContext, discordUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return account.ID, true, nil
}
