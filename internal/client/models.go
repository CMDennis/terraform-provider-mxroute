package client

// Response scalars use pointers: a missing value must never become false/zero
// during refresh. Nullable fields are explicitly marked for the strict decoder.
// Unknown additive API fields are tolerated. Units follow the pinned baseline.
type Record struct {
	Type        *string `json:"type"`
	Name        *string `json:"name"`
	Value       *string `json:"value"`
	Description *string `json:"description" optional:"true"`
}
type VerificationKey struct {
	Key         *string `json:"key"`
	Record      *Record `json:"record"`
	Description *string `json:"description" optional:"true"`
}
type Domain struct {
	Domain      *string  `json:"domain"`
	MailHosting *bool    `json:"mail_hosting"`
	SSLEnabled  *bool    `json:"ssl_enabled"`
	Pointers    []string `json:"pointers"`
}
type DomainCreated struct {
	Domain     *string `json:"domain"`
	SSLEnabled *bool   `json:"ssl_enabled"`
}
type DomainPointer struct {
	Pointer *string `json:"pointer"`
	Type    *string `json:"type"`
	Target  *string `json:"target"`
}
type EmailAccount struct {
	Username  *string  `json:"username"`
	Email     *string  `json:"email"`
	Quota     *int64   `json:"quota"` // MB; zero means unlimited.
	Usage     *float64 `json:"usage"` // MB.
	Limit     *int64   `json:"limit"`
	Sent      *int64   `json:"sent"`
	Suspended *bool    `json:"suspended"`
}
type Forwarder struct {
	Alias        *string  `json:"alias"`
	Email        *string  `json:"email"`
	Destinations []string `json:"destinations"`
}
type MXRecord struct {
	Priority    *int64  `json:"priority"`
	Hostname    *string `json:"hostname"`
	Description *string `json:"description" optional:"true"`
}
type DNSRecords struct {
	MXRecords    []MXRecord `json:"mx_records"`
	SPF          *Record    `json:"spf"`
	DKIM         *Record    `json:"dkim" nullable:"true"`
	Verification *Record    `json:"verification" nullable:"true"`
}
type CatchAll struct {
	Type        *string `json:"type"`
	Address     *string `json:"address" nullable:"true"`
	Description *string `json:"description" optional:"true"`
}
type SpamSettings struct {
	HighScore *int64 `json:"high_score"`
}
type QuotaBreakdown struct {
	Email     *int64 `json:"email"`
	Web       *int64 `json:"web"`
	Databases *int64 `json:"databases"`
	Backups   *int64 `json:"backups"`
	Other     *int64 `json:"other"`
}
type GracePeriod struct {
	DaysRemaining *int64  `json:"days_remaining"`
	Deadline      *string `json:"deadline"`
}
type Quota struct {
	Username    *string         `json:"username"`
	TotalUsed   *int64          `json:"total_used"`  // Bytes.
	TotalLimit  *int64          `json:"total_limit"` // Bytes; zero means unlimited.
	PercentUsed *float64        `json:"percent_used"`
	Breakdown   *QuotaBreakdown `json:"breakdown"`
	GracePeriod *GracePeriod    `json:"grace_period" nullable:"true"`
	UpdatedAt   *string         `json:"updated_at"`
}
type EmailUsageAccount struct {
	EmailAddress *string `json:"email_address"`
	SizeBytes    *int64  `json:"size_bytes"`
	UpdatedAt    *string `json:"updated_at"`
}
type EmailUsage struct {
	Username *string             `json:"username"`
	Accounts []EmailUsageAccount `json:"accounts"`
}
type UserQuota struct {
	Limit     *int64   `json:"limit" nullable:"true"`
	Used      *float64 `json:"used"`
	Unlimited *bool    `json:"unlimited"`
}
type ResellerUser struct {
	Username  *string    `json:"username"`
	Email     *string    `json:"email"`
	Domain    *string    `json:"domain"`
	Package   *string    `json:"package"`
	Suspended *bool      `json:"suspended"`
	Quota     *UserQuota `json:"quota"`
}
type PackageSettings struct {
	QuotaGB         *float64 `json:"quota_gb" nullable:"true"`
	QuotaUnlimited  *bool    `json:"quota_unlimited"`
	Domains         *int64   `json:"domains" nullable:"true"`
	EmailAccounts   *int64   `json:"email_accounts" nullable:"true"`
	EmailForwarders *int64   `json:"email_forwarders" nullable:"true"`
	DomainPointers  *int64   `json:"domain_pointers" nullable:"true"`
}
type ResellerPackage struct {
	Name     *string          `json:"name"`
	Settings *PackageSettings `json:"settings"`
}

// Request pointers distinguish omission from explicit zero, false or empty.
// Defaults are applied by the API, never filled in by the client.
type CreateDomainRequest struct {
	Domain string `json:"domain"`
}
type CreateDomainPointerRequest struct {
	Pointer string `json:"pointer"`
	Alias   *bool  `json:"alias,omitempty"`
}
type CreateEmailAccountRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Quota    *int64 `json:"quota,omitempty"`
	Limit    *int64 `json:"limit,omitempty"`
}
type UpdateEmailAccountRequest struct {
	Password *string `json:"password,omitempty"`
	Quota    *int64  `json:"quota,omitempty"`
	Limit    *int64  `json:"limit,omitempty"`
}
type CreateForwarderRequest struct {
	Alias        string   `json:"alias"`
	Destinations []string `json:"destinations"`
}
type SetCatchAllRequest struct {
	Type    string  `json:"type"`
	Address *string `json:"address,omitempty"`
}
type CreateResellerUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Package  string `json:"package"`
}
type UpdateResellerUserRequest struct {
	Quota    *string `json:"quota,omitempty"` // MB or "unlimited".
	Password *string `json:"password,omitempty"`
}
type PackageLimits struct {
	Quota           *string `json:"quota,omitempty"` // GB or "unlimited".
	Domains         *string `json:"domains,omitempty"`
	EmailAccounts   *string `json:"email_accounts,omitempty"`
	EmailForwarders *string `json:"email_forwarders,omitempty"`
	DomainPointers  *string `json:"domain_pointers,omitempty"`
}
type CreateResellerPackageRequest struct {
	Name string `json:"name"`
	PackageLimits
}

// Credential-bearing request formatting must never expose passwords.
func (CreateEmailAccountRequest) String() string     { return "MXroute mailbox request (redacted)" }
func (r CreateEmailAccountRequest) GoString() string { return r.String() }
func (UpdateEmailAccountRequest) String() string     { return "MXroute mailbox update (redacted)" }
func (r UpdateEmailAccountRequest) GoString() string { return r.String() }
func (CreateResellerUserRequest) String() string     { return "MXroute user request (redacted)" }
func (r CreateResellerUserRequest) GoString() string { return r.String() }
func (UpdateResellerUserRequest) String() string     { return "MXroute user update (redacted)" }
func (r UpdateResellerUserRequest) GoString() string { return r.String() }
