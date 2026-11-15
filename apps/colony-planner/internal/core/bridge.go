package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// The Foundry bridge (docs/design/foundry-bridge.md). A campaign has at most one active link to a
// Foundry world, made by trading a one-time pairing code for a bridge token. Colony changes write
// outbox rows built from the player projection in their own transaction; Foundry's events become
// status on the link or items in the DM's inbox, which change nothing until the DM acts.

const (
	// BridgePairingCodeLifetime and BridgePairingCodesPerHour bound the codes of one campaign.
	BridgePairingCodeLifetime = 10 * time.Minute
	BridgePairingCodesPerHour = 5
	// BridgeEventsPerBatch is the most events one POST /api/bridge/events may carry.
	BridgeEventsPerBatch = 100
	// BridgeInboxLimit caps a campaign's PENDING inbox items, so a leaked token can't flood the board.
	BridgeInboxLimit = 100
	// BridgeTimeAdvancedMaxDays is the largest TIME_ADVANCED event (ten years).
	BridgeTimeAdvancedMaxDays = 3650

	bridgeOutboxPageSize = 100
	bridgeOutboxLimit    = 10000
	// bridgeOutboxOneShotFloor one-shot rows always stay waiting, however many state rows do.
	bridgeOutboxOneShotFloor     = 1000
	bridgeDecidedEventLimit      = 10000
	bridgeRetention              = 30 * 24 * time.Hour
	bridgeStatusFieldMaxLength   = 100
	bridgeFailureReasonMaxLength = 300
	bridgeFailedRowsShown        = 20
	bridgePairingCodeLength      = 6
	bridgePairingCodeKeep        = 24 * time.Hour
	// bridgeLocalOrigin is a Foundry server on the author's own machine, accepted only in devauth builds.
	bridgeLocalOrigin       = "http://localhost:30000"
	bridgeColonyTarget      = "colony"
	bridgeAdvanceTargetHead = "advance:"
)

var (
	// bridgeTokenPattern is 32 random bytes in unpadded base64url.
	bridgeTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	// bridgeForgeOrigin is a Forge game's page: every game has its own subdomain, and custom domains
	// only redirect there, so this is the only origin a Forge-hosted module runs on.
	bridgeForgeOrigin = regexp.MustCompile(`^https://[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.forge-vtt\.com$`)
)

// BridgeCaller is a bridge request's principal: a live token of an ACTIVE campaign. Only core builds
// one (EnterBridge), its fields are unexported, and it is neither a Viewer nor a Caller, so bridge
// requests reach only the bridge operations. Its zero value names no link, and every bridge operation
// refuses it.
type BridgeCaller struct {
	campaignID uuid.UUID
	linkID     uuid.UUID
	origin     string
}

func (caller BridgeCaller) CampaignID() uuid.UUID { return caller.campaignID }

func (caller BridgeCaller) LinkID() uuid.UUID { return caller.linkID }

// Origin is the Forge page the link was made from; only that origin may read bridge answers.
func (caller BridgeCaller) Origin() string { return caller.origin }

// ErrBridgeUnauthorized is the one answer to a missing, malformed, revoked or unknown token, or one
// whose campaign isn't ACTIVE: 401 with no detail, so a guess learns nothing.
var ErrBridgeUnauthorized = &Error{Kind: KindSessionEnded}

// ErrBridgeCodeRejected answers a bad, used or expired pairing code alike.
var ErrBridgeCodeRejected = &Error{Kind: KindInvalid, Message: "That code didn't work. Make a new one on Colony Planner's Settings."}

// ErrBridgeOrigin answers a pairing request from anywhere but a Forge game's page.
var ErrBridgeOrigin = &Error{Kind: KindForbidden, Message: "Connect from your Foundry game's page on the Forge."}

func requireBridgeCaller(caller BridgeCaller) error {
	if caller.campaignID == uuid.Nil || caller.linkID == uuid.Nil {
		return ErrBridgeUnauthorized
	}
	return nil
}

// BridgePairingCode is a new code, shown once on Settings.
type BridgePairingCode struct {
	Code      string    `json:"code" doc:"e.g. K7Q-4XM; typed into the module's settings in Foundry"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// BridgeStatus is the Settings card (dm level).
type BridgeStatus struct {
	Link              *BridgeLinkStatus `json:"link" doc:"null while no Foundry world is connected"`
	Code              *BridgeOpenCode   `json:"code" doc:"The unused code, while one is open"`
	Inbox             []BridgeInboxItem `json:"inbox"`
	ModuleManifestURL string            `json:"moduleManifestUrl" doc:"What the DM pastes into Foundry's Install From Manifest"`
}

type BridgeLinkStatus struct {
	_              struct{}          `nullable:"true"`
	Origin         string            `json:"origin" doc:"e.g. https://myworld.forge-vtt.com"`
	LinkedAt       time.Time         `json:"linkedAt"`
	LastContactOn  *string           `json:"lastContactOn" format:"date" doc:"The last day the module called, UTC; null before its first call"`
	FoundryVersion string            `json:"foundryVersion" doc:"\"\" until the module's first status event, as are the other versions"`
	SystemID       string            `json:"systemId"`
	SystemVersion  string            `json:"systemVersion"`
	ModuleVersion  string            `json:"moduleVersion"`
	WorldTitle     string            `json:"worldTitle"`
	UnappliedRows  int64             `json:"unappliedRows"`
	FailedRows     []BridgeFailedRow `json:"failedRows" doc:"The newest rows Foundry couldn't apply, newest first"`
}

type BridgeFailedRow struct {
	Seq      int64            `json:"seq"`
	Kind     BridgeOutboxKind `json:"kind"`
	Reason   string           `json:"reason"`
	FailedAt time.Time        `json:"failedAt"`
}

type BridgeOpenCode struct {
	_         struct{}  `nullable:"true"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// BridgeInboxItem is one event waiting for the DM; its kind's own object is set and the others are null.
type BridgeInboxItem struct {
	ID         uuid.UUID              `json:"id" format:"uuid"`
	Kind       BridgeEventKind        `json:"kind"`
	ReceivedAt time.Time              `json:"receivedAt"`
	Days       int64                  `json:"days" doc:"A TIME_ADVANCED item's days; 0 for every other kind"`
	Deposit    *BridgeInboxDeposit    `json:"deposit" doc:"A COIN_DEPOSIT item's coins; null for every other kind"`
	Npc        *BridgeInboxNpc        `json:"npc" doc:"An NPC_IMPORT item's NPC; null for every other kind"`
	Item       *BridgeInboxMarketItem `json:"item" doc:"An ITEM_IMPORT item's catalog item; null for every other kind"`
}

type BridgePairInput struct {
	Code string `json:"code"`
}

// BridgePairing is the token, returned once.
type BridgePairing struct {
	Token        string `json:"token" doc:"Send it as Authorization: Bearer <token> on every bridge request"`
	CampaignName string `json:"campaignName"`
}

// BridgeOutboxRow is one thing Foundry should show; Payload is built from the player projection.
type BridgeOutboxRow struct {
	Seq       int64            `json:"seq"`
	Kind      BridgeOutboxKind `json:"kind"`
	TargetKey string           `json:"targetKey"`
	Payload   json.RawMessage  `json:"payload"`
}

type BridgeOutboxPage struct {
	Rows []BridgeOutboxRow `json:"rows"`
	More bool              `json:"more" doc:"More unapplied rows follow the last one"`
}

type BridgeAckInput struct {
	Through int64              `json:"through" doc:"Rows up to this seq not listed in failed are applied"`
	Failed  []BridgeAckFailure `json:"failed,omitempty"`
}

type BridgeAckFailure struct {
	Seq    int64  `json:"seq"`
	Reason string `json:"reason" doc:"At most 300 characters"`
}

type BridgeEventsInput struct {
	Events []BridgeInboundEvent `json:"events" doc:"1 to 100 events"`
}

type BridgeInboundEvent struct {
	ID         uuid.UUID       `json:"id" format:"uuid" doc:"The module's own ID; a repeat is ignored"`
	Kind       BridgeEventKind `json:"kind"`
	OccurredAt time.Time       `json:"occurredAt"`
	Payload    json.RawMessage `json:"payload"`
}

type BridgeEventsResult struct {
	Accepted   int64 `json:"accepted"`
	Duplicates int64 `json:"duplicates"`
}

// bridgeStatusPayload is a STATUS event's payload: the world's versions, stored on the link.
type bridgeStatusPayload struct {
	FoundryVersion string `json:"foundryVersion"`
	SystemID       string `json:"systemId"`
	SystemVersion  string `json:"systemVersion"`
	ModuleVersion  string `json:"moduleVersion"`
	WorldTitle     string `json:"worldTitle"`
}

type bridgeTimeAdvancedPayload struct {
	Days *int64 `json:"days"`
}

// bridgeColonySummary is the COLONY_SUMMARY payload: the colony as its players see it.
type bridgeColonySummary struct {
	CampaignName     string                  `json:"campaignName"`
	Day              int64                   `json:"day"`
	Resources        []bridgeSummaryResource `json:"resources"`
	Workforce        []bridgeSummaryRole     `json:"workforce"`
	NextShipmentDay  *int64                  `json:"nextShipmentDay"`
	NextGoodsShipDay *int64                  `json:"nextGoodsShipDay"`
	SiteURL          string                  `json:"siteUrl"`
	NextSession      *bridgeNextSession      `json:"nextSession"`
}

type bridgeSummaryResource struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
}

type bridgeSummaryRole struct {
	Role  string `json:"role"`
	Count int64  `json:"count"`
}

// bridgeDaysAdvanced is the DAYS_ADVANCED payload, written by Advance and never by its undo.
type bridgeDaysAdvanced struct {
	FromDay int64 `json:"fromDay"`
	ToDay   int64 `json:"toDay"`
	Days    int64 `json:"days"`
}

// BridgeOriginAllowed reports whether a module may pair from this origin: a Forge game's page, or a
// local Foundry in devauth builds (core.Options.AllowLocalFoundryOrigin).
func (service *Service) BridgeOriginAllowed(origin string) bool {
	return bridgeForgeOrigin.MatchString(origin) || (service.allowLocalFoundryOrigin && origin == bridgeLocalOrigin)
}

// EnterBridge is the one way to a BridgeCaller: one indexed read of the token's live link in an ACTIVE
// campaign. The link's last contact day is written at most once a day (Invariant 9).
func (service *Service) EnterBridge(requestContext context.Context, token string) (BridgeCaller, error) {
	if !bridgeTokenPattern.MatchString(token) {
		return BridgeCaller{}, ErrBridgeUnauthorized
	}
	tokenHash := sha256.Sum256([]byte(token))
	row, err := service.queries.GetBridgeLinkByTokenHash(requestContext, tokenHash[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return BridgeCaller{}, ErrBridgeUnauthorized
	}
	if err != nil {
		return BridgeCaller{}, err
	}
	link := row.BridgeLink
	today := utcDay(service.clock())
	if !link.LastContactOn.Valid || link.LastContactOn.Time.Before(today) {
		if err := service.queries.TouchBridgeLinkContact(requestContext, store.TouchBridgeLinkContactParams{
			CampaignID: link.CampaignID, ID: link.ID, Today: pgtype.Date{Time: today, Valid: true},
		}); err != nil {
			service.logger.WarnContext(requestContext, "could not record the bridge's last contact", slog.String("link_id", link.ID.String()), slog.Any("error", err))
		}
	}
	return BridgeCaller{campaignID: link.CampaignID, linkID: link.ID, origin: link.Origin}, nil
}

func utcDay(instant time.Time) time.Time {
	utc := instant.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

// CreateBridgePairingCode makes the campaign's one open code, replacing an unused one.
func (service *Service) CreateBridgePairingCode(requestContext context.Context, viewer Viewer) (BridgePairingCode, error) {
	if err := requireDm(viewer); err != nil {
		return BridgePairingCode{}, err
	}
	now := service.clock().UTC()
	for attempt := 0; ; attempt++ {
		code, err := newBridgePairingCode()
		if err != nil {
			return BridgePairingCode{}, err
		}
		var expiresAt time.Time
		err = service.inTransaction(requestContext, func(queries *store.Queries) error {
			if err := queries.PruneBridgePairingCodes(requestContext, store.PruneBridgePairingCodesParams{CampaignID: viewer.campaignID, Before: now.Add(-bridgePairingCodeKeep)}); err != nil {
				return err
			}
			recent, err := queries.CountBridgePairingCodesSince(requestContext, store.CountBridgePairingCodesSinceParams{CampaignID: viewer.campaignID, Since: now.Add(-time.Hour)})
			if err != nil {
				return err
			}
			if recent >= BridgePairingCodesPerHour {
				return Conflict(fmt.Sprintf("You made %d codes in the last hour. Try again later.", BridgePairingCodesPerHour))
			}
			if err := queries.CloseOpenBridgePairingCodes(requestContext, store.CloseOpenBridgePairingCodesParams{CampaignID: viewer.campaignID, Now: now}); err != nil {
				return err
			}
			codeHash := sha256.Sum256([]byte(code))
			expiresAt, err = queries.CreateBridgePairingCode(requestContext, store.CreateBridgePairingCodeParams{
				CampaignID: viewer.campaignID, CodeHash: codeHash[:], Now: now, ExpiresAt: now.Add(BridgePairingCodeLifetime),
			})
			return err
		})
		// Another campaign's code may hold the same six characters; a fresh draw almost never does.
		if isUniqueViolation(err, "bridge_pairing_code_hash_idx") && attempt < 3 {
			continue
		}
		if err != nil {
			return BridgePairingCode{}, err
		}
		service.broker.Publish(viewer.campaignID, TopicBridge)
		return BridgePairingCode{Code: formatBridgePairingCode(code), ExpiresAt: expiresAt.UTC()}, nil
	}
}

func isUniqueViolation(err error, constraint string) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505" && postgresError.ConstraintName == constraint
}

func newBridgePairingCode() (string, error) {
	var builder strings.Builder
	limit := big.NewInt(int64(len(linkCodeAlphabet)))
	for range bridgePairingCodeLength {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		builder.WriteByte(linkCodeAlphabet[index.Int64()])
	}
	return builder.String(), nil
}

func formatBridgePairingCode(code string) string {
	return code[:bridgePairingCodeLength/2] + "-" + code[bridgePairingCodeLength/2:]
}

// normalizeBridgePairingCode accepts the code as shown, in any case and spacing.
func normalizeBridgePairingCode(typed string) string {
	return strings.NewReplacer("-", "", " ", "").Replace(strings.ToUpper(strings.TrimSpace(typed)))
}

// PairBridge trades a pairing code for the campaign's bridge token. The origin is the request's Origin
// header, which a page script can't forge; the link answers only that origin. Pairing revokes the
// campaign's earlier link and queues the colony summary and every revealed place, NPC and quest, so the
// new world starts complete.
func (service *Service) PairBridge(requestContext context.Context, origin string, input BridgePairInput) (BridgePairing, error) {
	if !service.BridgeOriginAllowed(origin) {
		return BridgePairing{}, ErrBridgeOrigin
	}
	code := normalizeBridgePairingCode(input.Code)
	codeHash := sha256.Sum256([]byte(code))
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return BridgePairing{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	tokenHash := sha256.Sum256([]byte(token))
	now := service.clock().UTC()
	var campaignID, linkID uuid.UUID
	var revoked []uuid.UUID
	var campaignName string
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		claimed, err := queries.ClaimBridgePairingCode(requestContext, store.ClaimBridgePairingCodeParams{CodeHash: codeHash[:], Now: now})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBridgeCodeRejected
		}
		if err != nil {
			return err
		}
		campaignID = claimed
		// The colony lock first, as every colony change takes it before the link row, so the summary
		// below can't miss a change that is committing meanwhile.
		if _, err := queries.GetColonyForUpdate(requestContext, campaignID); err != nil {
			return campaignGone(err)
		}
		revoked, err = queries.RevokeBridgeLinks(requestContext, store.RevokeBridgeLinksParams{CampaignID: campaignID, Now: now})
		if err != nil {
			return err
		}
		outboxSeq, err := queries.CampaignBridgeOutboxSeq(requestContext, campaignID)
		if err != nil {
			return err
		}
		link, err := queries.CreateBridgeLink(requestContext, store.CreateBridgeLinkParams{
			CampaignID: campaignID, TokenHash: tokenHash[:], Origin: origin, Now: now, OutboxSeq: outboxSeq,
		})
		if err != nil {
			return err
		}
		linkID = link.ID
		header, err := queries.GetBoardHeader(requestContext, campaignID)
		if err != nil {
			return campaignGone(err)
		}
		campaignName = header.CampaignName
		if _, err := queries.DeleteUnappliedBridgeOutboxKinds(requestContext, store.DeleteUnappliedBridgeOutboxKindsParams{CampaignID: campaignID, Kinds: bridgeBackfilledKinds}); err != nil {
			return err
		}
		if _, err := service.enqueueColonySummary(requestContext, queries, campaignID); err != nil {
			return err
		}
		outbox, err := service.openBridgeOutbox(requestContext, queries, campaignID)
		if err != nil {
			return err
		}
		if err := outbox.backfill(); err != nil {
			return err
		}
		_, err = outbox.finish()
		return err
	})
	if err != nil {
		return BridgePairing{}, err
	}
	service.broker.closeBridgeStreams(campaignID)
	service.broker.Publish(campaignID, TopicBridge)
	service.logger.InfoContext(requestContext, "bridge linked", slog.String("campaign_id", campaignID.String()), slog.String("link_id", linkID.String()), slog.Int("revoked_links", len(revoked)))
	return BridgePairing{Token: token, CampaignName: campaignName}, nil
}

// RevokeBridge ends the campaign's link and its unused code; the outbox and events stay for the record.
func (service *Service) RevokeBridge(requestContext context.Context, viewer Viewer) error {
	if err := requireDm(viewer); err != nil {
		return err
	}
	now := service.clock().UTC()
	var revoked []uuid.UUID
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// The code before the link, the order PairBridge takes them in, so a pairing racing this can't deadlock.
		if err := queries.CloseOpenBridgePairingCodes(requestContext, store.CloseOpenBridgePairingCodesParams{CampaignID: viewer.campaignID, Now: now}); err != nil {
			return err
		}
		var err error
		revoked, err = queries.RevokeBridgeLinks(requestContext, store.RevokeBridgeLinksParams{CampaignID: viewer.campaignID, Now: now})
		return err
	})
	if err != nil {
		return err
	}
	service.broker.closeBridgeStreams(viewer.campaignID)
	service.broker.Publish(viewer.campaignID, TopicBridge)
	service.logger.InfoContext(requestContext, "bridge revoked", slog.String("campaign_id", viewer.campaignID.String()), slog.Int("revoked_links", len(revoked)))
	return nil
}

// GetBridgeStatus is the Settings card: the link, the open code, the inbox and the manifest address.
func (service *Service) GetBridgeStatus(requestContext context.Context, viewer Viewer) (BridgeStatus, error) {
	if err := requireDm(viewer); err != nil {
		return BridgeStatus{}, err
	}
	now := service.clock().UTC()
	status := BridgeStatus{ModuleManifestURL: service.publicURL + "/foundry/module.json"}
	link, err := service.queries.GetActiveBridgeLink(requestContext, viewer.campaignID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return BridgeStatus{}, err
	default:
		linkStatus, err := service.bridgeLinkStatus(requestContext, link)
		if err != nil {
			return BridgeStatus{}, err
		}
		status.Link = &linkStatus
	}
	expiresAt, err := service.queries.GetOpenBridgePairingCode(requestContext, store.GetOpenBridgePairingCodeParams{CampaignID: viewer.campaignID, Now: now})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return BridgeStatus{}, err
	default:
		status.Code = &BridgeOpenCode{ExpiresAt: expiresAt.UTC()}
	}
	status.Inbox, err = bridgeInbox(requestContext, service.queries, viewer.campaignID)
	if err != nil {
		return BridgeStatus{}, err
	}
	return status, nil
}

func (service *Service) bridgeLinkStatus(requestContext context.Context, link store.BridgeLink) (BridgeLinkStatus, error) {
	unapplied, err := service.queries.CountUnappliedBridgeOutbox(requestContext, link.CampaignID)
	if err != nil {
		return BridgeLinkStatus{}, err
	}
	failed, err := service.queries.ListFailedBridgeOutbox(requestContext, store.ListFailedBridgeOutboxParams{CampaignID: link.CampaignID, RowLimit: bridgeFailedRowsShown})
	if err != nil {
		return BridgeLinkStatus{}, err
	}
	status := BridgeLinkStatus{
		Origin:         link.Origin,
		LinkedAt:       link.LinkedAt.UTC(),
		FoundryVersion: link.FoundryVersion,
		SystemID:       link.SystemID,
		SystemVersion:  link.SystemVersion,
		ModuleVersion:  link.ModuleVersion,
		WorldTitle:     link.WorldTitle,
		UnappliedRows:  int64(unapplied),
		FailedRows:     make([]BridgeFailedRow, 0, len(failed)),
	}
	if link.LastContactOn.Valid {
		lastContact := link.LastContactOn.Time.Format(time.DateOnly)
		status.LastContactOn = &lastContact
	}
	for _, row := range failed {
		failedAt := time.Time{}
		if row.FailedAt != nil {
			failedAt = row.FailedAt.UTC()
		}
		status.FailedRows = append(status.FailedRows, BridgeFailedRow{Seq: row.Seq, Kind: row.Kind, Reason: row.FailureReason, FailedAt: failedAt})
	}
	return status, nil
}

// bridgeInbox is the campaign's PENDING items, oldest first: the Settings list and the board's dm card.
func bridgeInbox(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID) ([]BridgeInboxItem, error) {
	rows, err := queries.ListPendingBridgeEvents(requestContext, store.ListPendingBridgeEventsParams{CampaignID: campaignID, RowLimit: BridgeInboxLimit})
	if err != nil {
		return nil, err
	}
	inbox := make([]BridgeInboxItem, 0, len(rows))
	for _, row := range rows {
		item, err := toBridgeInboxItem(row)
		if err != nil {
			return nil, err
		}
		inbox = append(inbox, item)
	}
	return inbox, nil
}

// ClearBridgeFailedRows forgets the changes Foundry couldn't apply, once the DM has dealt with them.
func (service *Service) ClearBridgeFailedRows(requestContext context.Context, viewer Viewer) error {
	if err := requireDm(viewer); err != nil {
		return err
	}
	cleared, err := service.queries.ClearFailedBridgeOutbox(requestContext, viewer.campaignID)
	if err != nil {
		return err
	}
	service.broker.Publish(viewer.campaignID, TopicBridge)
	service.logger.InfoContext(requestContext, "bridge failed rows cleared", slog.String("campaign_id", viewer.campaignID.String()), slog.Int64("cleared_rows", cleared))
	return nil
}

// decideBridgeEvent moves one PENDING item of the given kinds on and returns what it stored. A missing
// item is 404, or the named body field's error when the ID came in a body (Invariant 11); a decided one
// is 409. A PENDING item of another kind is the body field's error, or 409 on a path of its own kind.
func decideBridgeEvent(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, eventID uuid.UUID, kinds []BridgeEventKind,
	status BridgeEventStatus, now time.Time, bodyField string,
) (store.DecideBridgeEventRow, error) {
	kindNames := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		kindNames = append(kindNames, string(kind))
	}
	decided, err := queries.DecideBridgeEvent(requestContext, store.DecideBridgeEventParams{CampaignID: campaignID, ID: eventID, Kinds: kindNames, Status: status, Now: now})
	if err == nil {
		return decided, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.DecideBridgeEventRow{}, err
	}
	const missing = "That Foundry inbox item doesn't exist."
	currentStatus, err := queries.GetBridgeEventStatus(requestContext, store.GetBridgeEventStatusParams{CampaignID: campaignID, ID: eventID})
	switch {
	case errors.Is(err, pgx.ErrNoRows) && bodyField != "":
		return store.DecideBridgeEventRow{}, Invalid(missing, FieldError{Field: bodyField, Message: missing})
	case errors.Is(err, pgx.ErrNoRows):
		return store.DecideBridgeEventRow{}, NotFound(missing)
	case err != nil:
		return store.DecideBridgeEventRow{}, err
	case currentStatus == BridgeEventStatusPending && bodyField != "":
		const wrongKind = "That Foundry inbox item isn't one this form answers."
		return store.DecideBridgeEventRow{}, Invalid(wrongKind, FieldError{Field: bodyField, Message: wrongKind})
	}
	return store.DecideBridgeEventRow{}, Conflict("That Foundry inbox item was already handled.")
}

// enqueueColonySummary writes the colony's COLONY_SUMMARY row when the campaign has an active link,
// inside the caller's transaction (Invariant 2), and reports whether it wrote one.
func (service *Service) enqueueColonySummary(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID) (bool, error) {
	return service.enqueueBridgeRow(requestContext, queries, campaignID, BridgeOutboxKindColonySummary, func(int64) string { return bridgeColonyTarget }, func() (any, error) {
		return service.colonySummary(requestContext, queries, campaignID)
	})
}

// withBridgeTopic adds the bridge topic to an operation's topics when it enqueued a row.
func withBridgeTopic(bridged bool, topics ...Topic) []Topic {
	if bridged {
		return append(topics, TopicBridge)
	}
	return topics
}

// enqueueDaysAdvanced writes Advance's one-shot DAYS_ADVANCED row, keyed by its own seq.
func (service *Service) enqueueDaysAdvanced(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, advanced bridgeDaysAdvanced) (bool, error) {
	return service.enqueueBridgeRow(requestContext, queries, campaignID, BridgeOutboxKindDaysAdvanced, func(seq int64) string {
		return bridgeAdvanceTargetHead + strconv.FormatInt(seq, 10)
	}, func() (any, error) { return advanced, nil })
}

// enqueueBridgeRow replaces the target's unapplied row with a new one, so each target has at most one
// waiting, and trims the oldest one-shot rows past the cap. Without an active link it writes nothing.
func (service *Service) enqueueBridgeRow(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, kind BridgeOutboxKind,
	targetKey func(seq int64) string, buildPayload func() (any, error),
) (bool, error) {
	wrote, err := service.insertBridgeRow(requestContext, queries, campaignID, kind, targetKey, buildPayload)
	if err != nil || !wrote {
		return false, err
	}
	return true, service.trimBridgeOutbox(requestContext, queries, campaignID)
}

// insertBridgeRow is enqueueBridgeRow without the cap.
func (service *Service) insertBridgeRow(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, kind BridgeOutboxKind,
	targetKey func(seq int64) string, buildPayload func() (any, error),
) (bool, error) {
	seq, err := queries.NextBridgeOutboxSeq(requestContext, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	payload, err := buildPayload()
	if err != nil {
		return false, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("core: encode a bridge payload: %w", err)
	}
	key := targetKey(seq)
	if _, err := queries.DeleteUnappliedBridgeOutboxTarget(requestContext, store.DeleteUnappliedBridgeOutboxTargetParams{CampaignID: campaignID, TargetKey: key}); err != nil {
		return false, err
	}
	if err := queries.CreateBridgeOutboxRow(requestContext, store.CreateBridgeOutboxRowParams{
		CampaignID: campaignID, Seq: seq, Kind: kind, TargetKey: key, Payload: encoded, Now: service.clock().UTC(),
	}); err != nil {
		return false, err
	}
	return true, nil
}

// insertBridgeRows writes a transaction's rows in three statements, with consecutive seqs in their
// order, after deleting the waiting rows of their targets. The targets are distinct (bridgeOutbox.write
// keeps one per target). Without an active link it writes nothing.
func (service *Service) insertBridgeRows(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, rows []bridgePendingRow) (bool, error) {
	lastSeq, err := queries.ReserveBridgeOutboxSeqs(requestContext, store.ReserveBridgeOutboxSeqsParams{CampaignID: campaignID, RowCount: int64(len(rows))})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	batch := store.CreateBridgeOutboxRowsParams{
		CampaignID: campaignID,
		Seqs:       make([]int64, 0, len(rows)),
		Kinds:      make([]string, 0, len(rows)),
		TargetKeys: make([]string, 0, len(rows)),
		Payloads:   make([]string, 0, len(rows)),
		Now:        service.clock().UTC(),
	}
	firstSeq := lastSeq - int64(len(rows)) + 1
	for index, row := range rows {
		batch.Seqs = append(batch.Seqs, firstSeq+int64(index))
		batch.Kinds = append(batch.Kinds, string(row.kind))
		batch.TargetKeys = append(batch.TargetKeys, row.targetKey)
		batch.Payloads = append(batch.Payloads, string(row.payload))
	}
	if _, err := queries.DeleteUnappliedBridgeOutboxTargets(requestContext, store.DeleteUnappliedBridgeOutboxTargetsParams{CampaignID: campaignID, TargetKeys: batch.TargetKeys}); err != nil {
		return false, err
	}
	if err := queries.CreateBridgeOutboxRows(requestContext, batch); err != nil {
		return false, err
	}
	return true, nil
}

// trimBridgeOutbox drops the oldest one-shot rows past the cap, keeping at least the newest
// bridgeOutboxOneShotFloor of them. State rows stay: each is its target's only waiting row, so
// trimming one would lose that target until it next changed.
func (service *Service) trimBridgeOutbox(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID) error {
	trimmed, err := queries.TrimBridgeOutbox(requestContext, store.TrimBridgeOutboxParams{
		CampaignID: campaignID, OneShotKinds: bridgeOneShotKinds, MinOneShots: bridgeOutboxOneShotFloor, Keep: bridgeOutboxLimit,
	})
	if err != nil {
		return err
	}
	if trimmed > 0 {
		service.logger.WarnContext(requestContext, "bridge outbox trimmed", slog.String("campaign_id", campaignID.String()), slog.Int64("rows", trimmed))
	}
	return nil
}

// colonySummary is built from the player projection, as Beny Bot's replies are, so no dm object or
// hidden row can reach Foundry (Invariant 15).
func (service *Service) colonySummary(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID) (bridgeColonySummary, error) {
	board, err := service.readBoard(requestContext, queries, campaignID, false)
	if err != nil {
		return bridgeColonySummary{}, err
	}
	header, err := queries.GetBoardHeader(requestContext, campaignID)
	if err != nil {
		return bridgeColonySummary{}, campaignGone(err)
	}
	openOrders, err := queries.ListOpenMarketOrders(requestContext, campaignID)
	if err != nil {
		return bridgeColonySummary{}, err
	}
	summary := bridgeColonySummary{
		CampaignName: board.CampaignName,
		Day:          board.Day,
		Resources:    make([]bridgeSummaryResource, 0, len(board.Resources)),
		Workforce:    make([]bridgeSummaryRole, 0, len(board.Roles)),
		SiteURL:      service.publicURL + "/c/" + header.CampaignSlug,
	}
	for _, resource := range board.Resources {
		summary.Resources = append(summary.Resources, bridgeSummaryResource{Name: resource.Label, Amount: resource.Amount})
	}
	for _, role := range board.Roles {
		summary.Workforce = append(summary.Workforce, bridgeSummaryRole{Role: role.Label, Count: role.Total})
	}
	if board.Shipments != nil {
		nextDay := board.Shipments.NextDay
		summary.NextShipmentDay = &nextDay
	}
	for _, order := range openOrders {
		goodsDay := int64(order.MarketOrder.ArrivesDay)
		if !game.Docked(board.Day, goodsDay) {
			summary.NextGoodsShipDay = &goodsDay
			break
		}
	}
	next, err := service.nextScheduledSession(requestContext, queries, campaignID)
	if err != nil {
		return bridgeColonySummary{}, err
	}
	if next != nil {
		summary.NextSession = &bridgeNextSession{
			Title: next.Title, StartsAt: next.StartsAt.UTC(), EndsAt: next.EndsAt.UTC(), LocalDate: next.LocalDate, Timezone: board.Timezone,
		}
	}
	return summary, nil
}

// ReadBridgeOutbox is up to 100 unapplied rows after the cursor, oldest first.
func (service *Service) ReadBridgeOutbox(requestContext context.Context, caller BridgeCaller, after int64) (BridgeOutboxPage, error) {
	if err := requireBridgeCaller(caller); err != nil {
		return BridgeOutboxPage{}, err
	}
	if after < 0 {
		return BridgeOutboxPage{}, Invalid("after must be 0 or more.", FieldError{Field: "after", Message: "Use 0 or more."})
	}
	rows, err := service.queries.ListBridgeOutbox(requestContext, store.ListBridgeOutboxParams{CampaignID: caller.campaignID, After: after, RowLimit: bridgeOutboxPageSize + 1})
	if err != nil {
		return BridgeOutboxPage{}, err
	}
	page := BridgeOutboxPage{Rows: make([]BridgeOutboxRow, 0, len(rows)), More: len(rows) > bridgeOutboxPageSize}
	for index, row := range rows {
		if index == bridgeOutboxPageSize {
			break
		}
		page.Rows = append(page.Rows, BridgeOutboxRow{Seq: row.Seq, Kind: row.Kind, TargetKey: row.TargetKey, Payload: row.Payload})
	}
	return page, nil
}

// NewestBridgeSeq is the newest unapplied row's seq, 0 when none waits: the bridge stream's one read
// on connect and after each bridge publish.
func (service *Service) NewestBridgeSeq(requestContext context.Context, caller BridgeCaller) (int64, error) {
	if err := requireBridgeCaller(caller); err != nil {
		return 0, err
	}
	return service.queries.NewestUnappliedBridgeSeq(requestContext, caller.campaignID)
}

// AckBridgeOutbox marks rows up to through applied, except the failed ones, which are kept with their
// reason for the DM and never offered again. Rows applied or failed over 30 days ago go here, lazily.
func (service *Service) AckBridgeOutbox(requestContext context.Context, caller BridgeCaller, input BridgeAckInput) error {
	if err := requireBridgeCaller(caller); err != nil {
		return err
	}
	if input.Through < 0 {
		return Invalid("through must be 0 or more.", FieldError{Field: "through", Message: "Use 0 or more."})
	}
	if len(input.Failed) > bridgeOutboxPageSize {
		return Invalid(fmt.Sprintf("List at most %d failed rows.", bridgeOutboxPageSize), FieldError{Field: "failed", Message: fmt.Sprintf("List at most %d rows.", bridgeOutboxPageSize)})
	}
	reasons := make(map[int64]string, len(input.Failed))
	for index, failure := range input.Failed {
		field := "failed[" + strconv.Itoa(index) + "]"
		if failure.Seq <= 0 {
			return Invalid("A failed row's seq must be 1 or more.", FieldError{Field: field + ".seq", Message: "Use 1 or more."})
		}
		reason, fieldError := cleanText(field+".reason", failure.Reason, bridgeFailureReasonMaxLength)
		if fieldError != nil {
			return Invalid(fieldError.Message, *fieldError)
		}
		reasons[failure.Seq] = reason
	}
	now := service.clock().UTC()
	var applied, failed, pruned int64
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Rows belong to the campaign, so a replaced link's late ack must not mark the new world's rows.
		if err := lockLiveBridgeLink(requestContext, queries, caller); err != nil {
			return err
		}
		for seq, reason := range reasons {
			marked, err := queries.FailBridgeOutboxRow(requestContext, store.FailBridgeOutboxRowParams{CampaignID: caller.campaignID, Seq: seq, FailureReason: reason, Now: now})
			if err != nil {
				return err
			}
			failed += marked
		}
		var err error
		applied, err = queries.ApplyBridgeOutboxThrough(requestContext, store.ApplyBridgeOutboxThroughParams{CampaignID: caller.campaignID, Through: input.Through, Now: now})
		if err != nil {
			return err
		}
		pruned, err = queries.PruneBridgeOutbox(requestContext, store.PruneBridgeOutboxParams{CampaignID: caller.campaignID, Before: now.Add(-bridgeRetention)})
		return err
	})
	if err != nil {
		return err
	}
	if applied > 0 || failed > 0 {
		service.broker.Publish(caller.campaignID, TopicBridge)
	}
	service.logger.InfoContext(requestContext, "bridge rows acknowledged", slog.String("link_id", caller.linkID.String()),
		slog.Int64("applied", applied), slog.Int64("failed", failed), slog.Int64("pruned", pruned))
	return nil
}

// validatedBridgeEvent is one inbound event that passed every check, ready to store.
type validatedBridgeEvent struct {
	event  BridgeInboundEvent
	status *bridgeStatusPayload
	days   *int32
	// gold and content are a Phase 3 inbox item's (bridge_imports.go): its amount and cleaned payload.
	gold    *int32
	content []byte
}

// RecordBridgeEvents stores a batch of Foundry's events. Every field is checked before anything is
// written, since any player can run code in their own browser; a repeated module ID is ignored. STATUS
// lands on the link; every other kind waits in the DM's inbox and changes nothing by itself.
func (service *Service) RecordBridgeEvents(requestContext context.Context, caller BridgeCaller, input BridgeEventsInput) (BridgeEventsResult, error) {
	if err := requireBridgeCaller(caller); err != nil {
		return BridgeEventsResult{}, err
	}
	if len(input.Events) == 0 || len(input.Events) > BridgeEventsPerBatch {
		message := fmt.Sprintf("Send 1 to %d events.", BridgeEventsPerBatch)
		return BridgeEventsResult{}, Invalid(message, FieldError{Field: "events", Message: message})
	}
	validated := make([]validatedBridgeEvent, 0, len(input.Events))
	inboxEvents := 0
	for index, event := range input.Events {
		checked, err := validateBridgeEvent("events["+strconv.Itoa(index)+"]", event)
		if err != nil {
			return BridgeEventsResult{}, err
		}
		if checked.status == nil {
			inboxEvents++
		}
		validated = append(validated, checked)
	}
	now := service.clock().UTC()
	var result BridgeEventsResult
	inboxChanged, statusChanged := false, false
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if err := lockLiveBridgeLink(requestContext, queries, caller); err != nil {
			return err
		}
		for _, checked := range validated {
			status := BridgeEventStatusAccepted
			if checked.status == nil {
				status = BridgeEventStatusPending
			}
			inserted, err := queries.CreateBridgeEvent(requestContext, store.CreateBridgeEventParams{
				CampaignID: caller.campaignID, ModuleEventID: checked.event.ID, Kind: checked.event.Kind, Status: status,
				OccurredAt: checked.event.OccurredAt.UTC(), Now: now, Days: checked.days, Gold: checked.gold, Content: checked.content,
			})
			if err != nil {
				return err
			}
			if inserted == 0 {
				result.Duplicates++
				continue
			}
			result.Accepted++
			if checked.status != nil {
				statusChanged = true
				if err := queries.SetBridgeLinkStatus(requestContext, store.SetBridgeLinkStatusParams{
					CampaignID: caller.campaignID, ID: caller.linkID,
					FoundryVersion: checked.status.FoundryVersion, SystemID: checked.status.SystemID, SystemVersion: checked.status.SystemVersion,
					ModuleVersion: checked.status.ModuleVersion, WorldTitle: checked.status.WorldTitle,
				}); err != nil {
					return err
				}
			} else {
				inboxChanged = true
			}
		}
		// Counted after the inserts, so a retried batch's repeats count as duplicates, not as new items.
		if inboxChanged {
			pending, err := queries.CountPendingBridgeEvents(requestContext, caller.campaignID)
			if err != nil {
				return err
			}
			if pending > BridgeInboxLimit {
				return Conflict(fmt.Sprintf("The DM's Foundry inbox holds its limit of %d items.", BridgeInboxLimit))
			}
		}
		if err := queries.PruneBridgeEvents(requestContext, store.PruneBridgeEventsParams{CampaignID: caller.campaignID, Before: now.Add(-bridgeRetention)}); err != nil {
			return err
		}
		return queries.TrimBridgeEvents(requestContext, store.TrimBridgeEventsParams{CampaignID: caller.campaignID, Keep: bridgeDecidedEventLimit})
	})
	if err != nil {
		return BridgeEventsResult{}, err
	}
	switch {
	case inboxChanged:
		service.broker.Publish(caller.campaignID, TopicColony, TopicBridge)
	case statusChanged:
		service.broker.Publish(caller.campaignID, TopicBridge)
	}
	service.logger.InfoContext(requestContext, "bridge events recorded", slog.String("link_id", caller.linkID.String()),
		slog.Int64("accepted", result.Accepted), slog.Int64("duplicates", result.Duplicates), slog.Int("inbox_events", inboxEvents))
	return result, nil
}

// lockLiveBridgeLink holds the caller's link for the transaction, and refuses one revoked since EnterBridge.
func lockLiveBridgeLink(requestContext context.Context, queries *store.Queries, caller BridgeCaller) error {
	_, err := queries.LockLiveBridgeLink(requestContext, store.LockLiveBridgeLinkParams{CampaignID: caller.campaignID, ID: caller.linkID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrBridgeUnauthorized
	}
	return err
}

func validateBridgeEvent(field string, event BridgeInboundEvent) (validatedBridgeEvent, error) {
	invalid := func(name, message string) (validatedBridgeEvent, error) {
		return validatedBridgeEvent{}, Invalid(message, FieldError{Field: field + "." + name, Message: message})
	}
	if event.ID == uuid.Nil {
		return invalid("id", "Give every event its own ID.")
	}
	if event.OccurredAt.IsZero() {
		return invalid("occurredAt", "Say when the event happened.")
	}
	checked := validatedBridgeEvent{event: event}
	switch event.Kind {
	case BridgeEventKindStatus:
		var payload bridgeStatusPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return invalid("payload", "A STATUS payload holds five strings.")
		}
		for _, value := range []struct {
			name   string
			target *string
		}{
			{"foundryVersion", &payload.FoundryVersion},
			{"systemId", &payload.SystemID},
			{"systemVersion", &payload.SystemVersion},
			{"moduleVersion", &payload.ModuleVersion},
			{"worldTitle", &payload.WorldTitle},
		} {
			cleaned, fieldError := cleanText(field+".payload."+value.name, *value.target, bridgeStatusFieldMaxLength)
			if fieldError != nil {
				return validatedBridgeEvent{}, Invalid(fieldError.Message, *fieldError)
			}
			*value.target = cleaned
		}
		checked.status = &payload
	case BridgeEventKindTimeAdvanced:
		var payload bridgeTimeAdvancedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.Days == nil ||
			*payload.Days < 1 || *payload.Days > BridgeTimeAdvancedMaxDays {
			return invalid("payload.days", fmt.Sprintf("Use a whole number of days from 1 to %d.", BridgeTimeAdvancedMaxDays))
		}
		days := int32(*payload.Days)
		checked.days = &days
	default:
		return validateBridgeImport(field, event)
	}
	return checked, nil
}

// bridgeInboxForBoard fills the board's dm card; players' boards skip the read.
func bridgeInboxForBoard(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, dmProjection bool) ([]BridgeInboxItem, error) {
	if !dmProjection {
		return []BridgeInboxItem{}, nil
	}
	return bridgeInbox(requestContext, queries, campaignID)
}

// SubscribeBridge opens the bridge stream's subscription: it hears only the campaign's bridge topic,
// and revoking or replacing the link, suspending or deleting the campaign closes it.
func (service *Service) SubscribeBridge(requestContext context.Context, caller BridgeCaller) (*Subscription, error) {
	if err := requireBridgeCaller(caller); err != nil {
		return nil, err
	}
	subscription := service.broker.subscribeBridge(caller.campaignID)
	if subscription == nil {
		return nil, ErrBridgeUnauthorized
	}
	// A revoke between EnterBridge and the subscription would otherwise miss closing this stream.
	link, err := service.queries.GetActiveBridgeLink(requestContext, caller.campaignID)
	if err != nil || link.ID != caller.linkID {
		subscription.Close()
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBridgeUnauthorized
		}
		return nil, err
	}
	return subscription, nil
}
