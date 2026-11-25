package billingapp

import (
	"context"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/billing"
)

// UnbilledInput asks for the sessions of the practice that no issued voucher bills, inside a
// window, optionally narrowed to one payer (T73).
type UnbilledInput struct {
	TenantID string
	From     time.Time
	To       time.Time
	// ClientID narrows the worklist to the patients that pay through one client. Empty
	// means every payer of the practice.
	ClientID string
	Limit    int32
}

// UnbilledSessionView is one row of the worklist: who was seen, who pays for them (S26, T73),
// by whom and when. The amount is not here on purpose: pricing a session needs the tariff in
// force on its day and the payer's terms, and the worklist exists to say what is missing, not
// to quote it. The emission form shows the proposal for the session it is about to bill.
type UnbilledSessionView struct {
	ID               string
	PatientID        string `json:"patientId"`
	PatientName      string `json:"patientName"`
	ClientID         string `json:"clientId"`
	ClientName       string `json:"clientName"`
	ProfessionalID   string `json:"professionalId"`
	ProfessionalName string `json:"professionalName"`
	Modality         string `json:"modality"`
	StartsAt         string `json:"startsAt"`
}

// UnbilledUseCase answers what is owed a receipt and still has none.
type UnbilledUseCase struct {
	billing port.BillingRepository
	scope   port.TenantScope
}

func NewUnbilledUseCase(repository port.BillingRepository, scope port.TenantScope) *UnbilledUseCase {
	return &UnbilledUseCase{billing: repository, scope: scope}
}

func (uc *UnbilledUseCase) Execute(ctx context.Context, in UnbilledInput) ([]UnbilledSessionView, error) {
	if !in.To.After(in.From) {
		return nil, &billing.ValidationError{Field: "range", Message: "to must be after from"}
	}

	limit := in.Limit
	if limit == 0 {
		limit = DefaultSessionPageSize
	}
	if limit < 1 || limit > MaxSessionPageSize {
		return nil, &billing.ValidationError{Field: "limit", Message: "must be between 1 and 100"}
	}

	var views []UnbilledSessionView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		sessions, err := uc.billing.UnbilledSessions(ctx, port.UnbilledFilter{
			From:     in.From,
			To:       in.To,
			ClientID: in.ClientID,
			Limit:    limit,
		})
		if err != nil {
			return err
		}
		views = make([]UnbilledSessionView, 0, len(sessions))
		for _, session := range sessions {
			views = append(views, UnbilledSessionView{
				ID:               session.ID,
				PatientID:        session.PatientID,
				PatientName:      session.PatientName,
				ClientID:         session.ClientID,
				ClientName:       session.ClientName,
				ProfessionalID:   session.ProfessionalID,
				ProfessionalName: session.ProfessionalName,
				Modality:         session.Modality,
				StartsAt:         session.StartsAt.UTC().Format(time.RFC3339),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}
