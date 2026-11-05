// Package quote obtains live quotes from the provider and stores immutable snapshots bound
// to the exact payment parameters.
package quote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/amount"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/catalog"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/ids"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
)

const (
	ProviderNEARIntents = "near-intents-1click"
	// DefaultTTL is the deadline we request from 1Click. NEAR and Base deposits confirm in
	// seconds, so ten minutes leaves room for review and wallet approval.
	DefaultTTL = 10 * time.Minute
	// slippageBps is the slippage tolerance sent to 1Click (100 = 1%).
	slippageBps = 100
)

type CreateInput struct {
	SourceNetwork      string `json:"source_network"`
	SourceAssetID      string `json:"source_asset_id"`
	SourceAmount       string `json:"source_amount"`
	DestinationNetwork string `json:"destination_network"`
	DestinationAssetID string `json:"destination_asset_id"`
	DestinationAddress string `json:"destination_address"`
	RefundAddress      string `json:"refund_address"`
}

type Quote struct {
	ID                    string    `json:"quote_id"`
	RouteID               string    `json:"route_id"`
	Fingerprint           string    `json:"fingerprint"`
	SourceNetwork         string    `json:"source_network"`
	SourceAssetID         string    `json:"source_asset_id"`
	SourceAmount          string    `json:"source_amount"`
	DestinationNetwork    string    `json:"destination_network"`
	DestinationAssetID    string    `json:"destination_asset_id"`
	DestinationAddress    string    `json:"destination_address"`
	RefundAddress         string    `json:"refund_address"`
	ExpectedAmountOut     string    `json:"expected_destination_amount"`
	MinAmountOut          string    `json:"min_destination_amount"`
	DepositAddress        string    `json:"deposit_address"`
	DepositMemo           string    `json:"deposit_memo,omitempty"`
	TimeEstimateSeconds   *int      `json:"time_estimate_seconds,omitempty"`
	ExpiresAt             time.Time `json:"expires_at"`
	Provider              string    `json:"provider"`
	ProviderCorrelationID string    `json:"provider_correlation_id"`
	ProviderSignature     string    `json:"provider_signature"`
	CreatedAt             time.Time `json:"created_at"`

	WorkspaceID string `json:"-"`
}

// Expired reports whether the quote can no longer back a payment at time now.
func (q *Quote) Expired(now time.Time) bool { return !now.Before(q.ExpiresAt) }

type Service struct {
	DB       *pgxpool.Pool
	Provider intents.Provider
	Catalog  *catalog.Catalog
	Now      func() time.Time
}

// Fingerprint hashes the normalized payment parameters. Struct field order makes the JSON
// encoding deterministic. Any material change (amount, recipient, asset, network, refund
// address) produces a different fingerprint.
func Fingerprint(routeID string, in CreateInput) string {
	payload := struct {
		Version            int    `json:"v"`
		RouteID            string `json:"route_id"`
		SourceNetwork      string `json:"source_network"`
		SourceAssetID      string `json:"source_asset_id"`
		SourceAmount       string `json:"source_amount"`
		DestinationNetwork string `json:"destination_network"`
		DestinationAssetID string `json:"destination_asset_id"`
		DestinationAddress string `json:"destination_address"`
		RefundAddress      string `json:"refund_address"`
	}{1, routeID, in.SourceNetwork, in.SourceAssetID, in.SourceAmount, in.DestinationNetwork,
		in.DestinationAssetID, in.DestinationAddress, in.RefundAddress}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Validate checks input against the catalogue and returns the matching route.
func (s *Service) Validate(in CreateInput) (catalog.Route, error) {
	if !s.Catalog.HasNetwork(in.SourceNetwork) || !s.Catalog.HasNetwork(in.DestinationNetwork) {
		return catalog.Route{}, apperr.New(http.StatusUnprocessableEntity, apperr.CodeUnsupportedNetwork, "Source or destination network is not supported.")
	}
	if !s.Catalog.HasAsset(in.SourceNetwork, in.SourceAssetID) || !s.Catalog.HasAsset(in.DestinationNetwork, in.DestinationAssetID) {
		return catalog.Route{}, apperr.New(http.StatusUnprocessableEntity, apperr.CodeUnsupportedAsset, "Asset is not supported on that network. Use canonical asset IDs from GET /api/v1/assets.")
	}
	route, ok := s.Catalog.FindRoute(in.SourceNetwork, in.SourceAssetID, in.DestinationNetwork, in.DestinationAssetID)
	if !ok {
		return catalog.Route{}, apperr.New(http.StatusUnprocessableEntity, apperr.CodeRouteUnavailable, "No configured route between these assets.")
	}
	if _, err := amount.ParseBaseUnits(in.SourceAmount); err != nil {
		return catalog.Route{}, apperr.Validation("source_amount: " + err.Error())
	}
	if err := catalog.ValidateAddress(in.DestinationNetwork, in.DestinationAddress); err != nil {
		return catalog.Route{}, apperr.Validation("destination_address: " + err.Error())
	}
	if err := catalog.ValidateAddress(in.SourceNetwork, in.RefundAddress); err != nil {
		return catalog.Route{}, apperr.Validation("refund_address: " + err.Error())
	}
	return route, nil
}

// Create requests a live, executable quote and stores it.
func (s *Service) Create(ctx context.Context, workspaceID string, in CreateInput) (*Quote, error) {
	route, err := s.Validate(in)
	if err != nil {
		return nil, err
	}

	now := s.Now().UTC()
	req := intents.QuoteRequest{
		Dry:               false,
		SwapType:          "EXACT_INPUT",
		SlippageTolerance: slippageBps,
		OriginAsset:       route.Source.ID,
		DepositType:       route.DepositType,
		DestinationAsset:  route.Destination.ID,
		Amount:            in.SourceAmount,
		RefundTo:          in.RefundAddress,
		RefundType:        route.RefundType,
		Recipient:         in.DestinationAddress,
		RecipientType:     route.RecipientType,
		Deadline:          now.Add(DefaultTTL).Format(time.RFC3339Nano),
	}
	resp, err := s.Provider.Quote(ctx, req)
	if err != nil {
		return nil, providerError(err)
	}

	// Use the provider's deadline when given; never extend beyond what we asked for.
	expires := now.Add(DefaultTTL)
	if d := resp.Quote.Deadline; d != nil && d.Before(expires) {
		expires = d.UTC()
	}
	if !now.Before(expires) {
		return nil, apperr.New(http.StatusBadGateway, apperr.CodeProviderInvalidResp, "Provider returned a quote that is already expired.")
	}

	q := &Quote{
		ID:                    ids.New(ids.Quote),
		WorkspaceID:           workspaceID,
		RouteID:               route.ID,
		Fingerprint:           Fingerprint(route.ID, in),
		SourceNetwork:         in.SourceNetwork,
		SourceAssetID:         in.SourceAssetID,
		SourceAmount:          in.SourceAmount,
		DestinationNetwork:    in.DestinationNetwork,
		DestinationAssetID:    in.DestinationAssetID,
		DestinationAddress:    in.DestinationAddress,
		RefundAddress:         in.RefundAddress,
		ExpectedAmountOut:     resp.Quote.AmountOut,
		MinAmountOut:          resp.Quote.MinAmountOut,
		DepositAddress:        resp.Quote.DepositAddress,
		DepositMemo:           resp.Quote.DepositMemo,
		ExpiresAt:             expires,
		Provider:              ProviderNEARIntents,
		ProviderCorrelationID: resp.CorrelationID,
		ProviderSignature:     resp.Signature,
		// Timestamps come from the service clock, the same one used for expiry checks.
		CreatedAt: now,
	}
	if resp.Quote.TimeEstimate > 0 {
		secs := int(resp.Quote.TimeEstimate)
		q.TimeEstimateSeconds = &secs
	}
	reqJSON, _ := json.Marshal(req)

	_, err = s.DB.Exec(ctx, `
		INSERT INTO quotes (id, workspace_id, route_id, fingerprint, source_network, source_asset_id,
			source_amount, destination_network, destination_asset_id, destination_address, refund_address,
			expected_amount_out, min_amount_out, deposit_address, deposit_memo, time_estimate_seconds,
			expires_at, provider, provider_correlation_id, provider_signature, provider_request, provider_response,
			created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7::numeric,$8,$9,$10,$11,NULLIF($12,'')::numeric,NULLIF($13,'')::numeric,
			$14,NULLIF($15,''),$16,$17,$18,$19,$20,$21,$22,$23)`,
		q.ID, q.WorkspaceID, q.RouteID, q.Fingerprint, q.SourceNetwork, q.SourceAssetID,
		q.SourceAmount, q.DestinationNetwork, q.DestinationAssetID, q.DestinationAddress, q.RefundAddress,
		q.ExpectedAmountOut, q.MinAmountOut, q.DepositAddress, q.DepositMemo, q.TimeEstimateSeconds,
		q.ExpiresAt, q.Provider, q.ProviderCorrelationID, q.ProviderSignature, reqJSON, []byte(resp.Raw),
		q.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert quote: %w", err)
	}
	return q, nil
}

const selectCols = `id, workspace_id, route_id, fingerprint, source_network, source_asset_id,
	source_amount::text, destination_network, destination_asset_id, destination_address, refund_address,
	COALESCE(expected_amount_out::text, ''), COALESCE(min_amount_out::text, ''), COALESCE(deposit_address, ''),
	COALESCE(deposit_memo, ''), time_estimate_seconds, expires_at, provider,
	COALESCE(provider_correlation_id, ''), COALESCE(provider_signature, ''), created_at`

// Scan reads a quote row selected with SelectColumns.
func Scan(row pgx.Row) (*Quote, error) {
	var q Quote
	err := row.Scan(&q.ID, &q.WorkspaceID, &q.RouteID, &q.Fingerprint, &q.SourceNetwork, &q.SourceAssetID,
		&q.SourceAmount, &q.DestinationNetwork, &q.DestinationAssetID, &q.DestinationAddress, &q.RefundAddress,
		&q.ExpectedAmountOut, &q.MinAmountOut, &q.DepositAddress, &q.DepositMemo, &q.TimeEstimateSeconds,
		&q.ExpiresAt, &q.Provider, &q.ProviderCorrelationID, &q.ProviderSignature, &q.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.New(http.StatusNotFound, apperr.CodeQuoteNotFound, "Quote not found.")
	}
	if err != nil {
		return nil, fmt.Errorf("scan quote: %w", err)
	}
	return &q, nil
}

// SelectColumns is the column list Scan expects.
func SelectColumns() string { return selectCols }

func (s *Service) Get(ctx context.Context, workspaceID, id string) (*Quote, error) {
	return Scan(s.DB.QueryRow(ctx, `SELECT `+selectCols+` FROM quotes WHERE id = $1 AND workspace_id = $2`, id, workspaceID))
}

func providerError(err error) error {
	var pe *intents.Error
	if !errors.As(err, &pe) {
		return fmt.Errorf("provider quote: %w", err)
	}
	switch pe.Kind {
	case intents.KindRejected:
		return apperr.New(http.StatusUnprocessableEntity, apperr.CodeProviderRejected, "Provider rejected the quote request: "+pe.Message)
	case intents.KindInvalidResponse:
		return apperr.New(http.StatusBadGateway, apperr.CodeProviderInvalidResp, "Provider returned a quote that failed validation; it was not stored.")
	default:
		// Quotes do not move value, so the client may simply ask again.
		return apperr.New(http.StatusServiceUnavailable, apperr.CodeProviderUnavailable, "Provider is unavailable. Request a new quote.")
	}
}
