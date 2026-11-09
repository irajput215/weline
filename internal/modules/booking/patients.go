package booking

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

// Patients are upstream contacts, linked by id: booking stores no name or
// phone of its own (design.md, "Patients and contacts"). Phone numbers are
// stored the way upstream stores them: digits, country code first, no "+".

var nonDigits = regexp.MustCompile(`\D`)

// normaliseMobile turns an Australian mobile as typed (0412 345 678,
// +61 412 345 678, 61412345678) into 61412345678 (Q7).
func normaliseMobile(s string) (string, error) {
	d := nonDigits.ReplaceAllString(s, "")
	switch {
	case len(d) == 10 && strings.HasPrefix(d, "04"):
		d = "61" + d[1:]
	case len(d) == 11 && strings.HasPrefix(d, "614"):
	default:
		return "", kit.Invalid("enter an Australian mobile number, e.g. 0412 345 678")
	}
	return d, nil
}

// phoneDigits returns the digits of a phone number as upstream stores it.
func phoneDigits(s string) string { return strings.TrimPrefix(nonDigits.ReplaceAllString(s, ""), "+") }

// patient checks a contact belongs to the organization.
func (s *Service) patient(ctx context.Context, orgID, contactID uuid.UUID) (*Patient, error) {
	m, err := s.patients(ctx, orgID, []uuid.UUID{contactID})
	if err != nil {
		return nil, err
	}
	p, ok := m[contactID]
	if !ok {
		return nil, kit.NotFound("patient not found")
	}
	return &p, nil
}

func (s *Service) patients(ctx context.Context, orgID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]Patient, error) {
	out := map[uuid.UUID]Patient{}
	if len(ids) == 0 {
		return out, nil
	}
	var cs []models.Contact
	if err := s.db.WithContext(ctx).Select("id", "profile_name", "phone_number").
		Where("organization_id = ? AND id IN ?", orgID, ids).Find(&cs).Error; err != nil {
		return nil, fmt.Errorf("load patients: %w", err)
	}
	for _, c := range cs {
		out[c.ID] = Patient{ID: c.ID, Name: c.ProfileName, Phone: c.PhoneNumber}
	}
	return out, nil
}

// SearchPatients finds the organization's contacts by name or number (R13).
func (s *Service) SearchPatients(ctx context.Context, orgID uuid.UUID, q string) ([]Patient, error) {
	q = strings.TrimSpace(q)
	if len([]rune(q)) < 3 {
		return nil, kit.Invalid("type at least 3 characters")
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
	db := s.db.WithContext(ctx).Select("id", "profile_name", "phone_number").Where("organization_id = ?", orgID)
	if digits := nonDigits.ReplaceAllString(q, ""); len(digits) >= 3 && len(digits) >= len(q)/2 {
		if strings.HasPrefix(digits, "0") {
			digits = "61" + digits[1:] // local form → stored form
		}
		db = db.Where("phone_number LIKE ?", "%"+digits+"%")
	} else {
		db = db.Where("profile_name ILIKE ?", like)
	}
	var cs []models.Contact
	if err := db.Order("profile_name").Limit(20).Find(&cs).Error; err != nil {
		return nil, fmt.Errorf("search patients: %w", err)
	}
	out := make([]Patient, 0, len(cs))
	for _, c := range cs {
		out = append(out, Patient{ID: c.ID, Name: c.ProfileName, Phone: c.PhoneNumber})
	}
	return out, nil
}

// PatientByPhone finds the contact with this number in the organization, the
// most recently active one if there are several (one per WhatsApp account).
func (s *Service) PatientByPhone(ctx context.Context, orgID uuid.UUID, phone string) (*Patient, error) {
	d := phoneDigits(phone)
	if d == "" {
		return nil, kit.Invalid("phone is required")
	}
	var cs []models.Contact
	if err := s.db.WithContext(ctx).Select("id", "profile_name", "phone_number").
		Where("organization_id = ? AND phone_number IN ?", orgID, []string{d, "+" + d}).
		Order("last_message_at DESC NULLS LAST, created_at DESC").Limit(1).Find(&cs).Error; err != nil {
		return nil, fmt.Errorf("find patient: %w", err)
	}
	if len(cs) == 0 {
		return nil, kit.NotFound("we couldn't find your details; please message the clinic")
	}
	return &Patient{ID: cs[0].ID, Name: cs[0].ProfileName, Phone: cs[0].PhoneNumber}, nil
}

// FindOrCreatePatient returns the contact with this mobile, or adds one on the
// organization's default WhatsApp account (Q1). existing says which.
func (s *Service) FindOrCreatePatient(ctx context.Context, orgID uuid.UUID, name, mobile string) (*Patient, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false, kit.Invalid("name is required")
	}
	if len([]rune(name)) > 120 {
		return nil, false, kit.Invalid("name is too long (max 120 characters)")
	}
	phone, err := normaliseMobile(mobile)
	if err != nil {
		return nil, false, err
	}
	if p, err := s.PatientByPhone(ctx, orgID, phone); err == nil {
		return p, true, nil
	}
	var accounts []models.WhatsAppAccount
	if err := s.db.WithContext(ctx).Select("name").Where("organization_id = ?", orgID).
		Order("is_default_outgoing DESC, created_at").Limit(1).Find(&accounts).Error; err != nil {
		return nil, false, fmt.Errorf("load WhatsApp account: %w", err)
	}
	if len(accounts) == 0 {
		return nil, false, kit.Conflict("the clinic has no WhatsApp account set up yet")
	}
	c := models.Contact{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: orgID, PhoneNumber: phone,
		ProfileName: name, WhatsAppAccount: accounts[0].Name}
	if err := s.db.WithContext(ctx).Create(&c).Error; err != nil {
		return nil, false, fmt.Errorf("add patient: %w", err)
	}
	return &Patient{ID: c.ID, Name: c.ProfileName, Phone: c.PhoneNumber}, false, nil
}
