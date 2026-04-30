package client

type Page struct {
	Next string `json:"next"`
}

type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role"`
	Status    string `json:"status"`
}

type UsersList struct {
	Page  Page    `json:"page"`
	Users []*User `json:"data"`
}

type UsersResponse struct {
	Users      []*User
	Pagination string
}

type DeferredTaskResponse struct {
	ID string `json:"id"`
}

type Role struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Money mirrors Ramp's Money shape. Ramp returns major-unit amounts as JSON
// numbers (e.g. 48000.00) plus a minor_unit_conversion_rate. Convert to baton's
// int64 minor units via Money.MinorUnits().
type Money struct {
	Amount                  float64 `json:"amount"`
	CurrencyCode            string  `json:"currency_code"`
	MinorUnitConversionRate int     `json:"minor_unit_conversion_rate"`
}

// MinorUnits converts the Ramp Money to int64 minor units, rounding to the
// nearest unit. Returns nil when the Money has no currency_code (i.e. the
// API omitted the field).
func (m *Money) MinorUnits() int64 {
	if m == nil || m.CurrencyCode == "" {
		return 0
	}
	rate := m.MinorUnitConversionRate
	if rate <= 0 {
		// Defensive: spec says rate is always set, but never trust the wire.
		rate = 100
	}
	// math.Round avoids truncation; e.g. 48000.005 → 4800001 not 4800000.
	v := m.Amount * float64(rate)
	if v >= 0 {
		return int64(v + 0.5)
	}
	return int64(v - 0.5)
}

type Vendor struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	NameLegal                string `json:"name_legal"`
	IsActive                 bool   `json:"is_active"`
	IsDeletable              bool   `json:"is_deletable"`
	VendorOwnerID            string `json:"vendor_owner_id"`
	ExternalVendorID         string `json:"external_vendor_id"`
	DefaultEntityID          string `json:"default_entity_id"`
	AccountingVendorRemoteID string `json:"accounting_vendor_remote_id"`
	MerchantID               string `json:"merchant_id"`
	Description              string `json:"description"`
	VendorType               string `json:"vendor_type"`
	BillingFrequency         string `json:"billing_frequency"`
	Country                  string `json:"country"`
	State                    string `json:"state"`
	Subsidiary               []string `json:"subsidiary"`
	Contacts                 []string `json:"contacts"`
	CreatedAt                string `json:"created_at"`
	TotalSpendAllTime        *Money `json:"total_spend_all_time"`
	TotalSpendLast30Days     *Money `json:"total_spend_last_30_days"`
	TotalSpendLast365Days    *Money `json:"total_spend_last_365_days"`
	TotalSpendYTD            *Money `json:"total_spend_ytd"`
}

type VendorsList struct {
	Page    Page      `json:"page"`
	Vendors []*Vendor `json:"data"`
}

type VendorsResponse struct {
	Vendors    []*Vendor
	Pagination string
}
