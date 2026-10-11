package core

import (
	"cmp"
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/store"
)

// LoginOptions tells /login which sign-in methods exist. It is public, so it carries nothing a
// visitor may not see; the member list appears only in devauth builds.
type LoginOptions struct {
	DiscordEnabled bool              `json:"discordEnabled"`
	DevSignIn      []DevSignInMember `json:"devSignIn,omitempty" doc:"Present only in devauth builds"`
}

type DevSignInMember struct {
	MemberID      uuid.UUID    `json:"memberId" format:"uuid"`
	DisplayName   string       `json:"displayName"`
	CharacterName *string      `json:"characterName"`
	Status        MemberStatus `json:"status"`
	IsDM          bool         `json:"isDm"`
}

type LoginOptionsInput struct {
	DiscordEnabled bool
	// DevSignIn is true only in devauth builds; then every member is listed for one-click sign-in.
	DevSignIn bool
}

func (service *Service) LoginOptions(requestContext context.Context, input LoginOptionsInput) (LoginOptions, error) {
	options := LoginOptions{DiscordEnabled: input.DiscordEnabled}
	if !input.DevSignIn {
		return options, nil
	}
	members, err := service.DevSignInMembers(requestContext)
	if err != nil {
		return LoginOptions{}, err
	}
	options.DevSignIn = members
	return options, nil
}

var devSignInStatusOrder = map[store.MemberStatus]int{
	store.MemberStatusACTIVE:  0,
	store.MemberStatusPENDING: 1,
	store.MemberStatusREMOVED: 2,
}

// DevSignInMembers lists every member for dev sign-in: the DM first, then players, the waiting and
// the removed, each oldest first. Only devauth builds call it.
func (service *Service) DevSignInMembers(requestContext context.Context) ([]DevSignInMember, error) {
	rows, err := service.queries.ListMembers(requestContext)
	if err != nil {
		return nil, err
	}
	members := make([]DevSignInMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, DevSignInMember{
			MemberID:      row.ID,
			DisplayName:   row.DisplayName,
			CharacterName: row.CharacterName,
			Status:        service.effectiveStatus(row),
			IsDM:          service.isDM(row.DiscordUserID),
		})
	}
	// ListMembers is already oldest first, so a stable sort keeps that order within each group.
	slices.SortStableFunc(members, func(first, second DevSignInMember) int {
		if first.IsDM != second.IsDM {
			if first.IsDM {
				return -1
			}
			return 1
		}
		return cmp.Compare(devSignInStatusOrder[first.Status], devSignInStatusOrder[second.Status])
	})
	return members, nil
}
