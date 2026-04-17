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

type Vendor struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Code          string `json:"code"`
	IsActive      bool   `json:"is_active"`
	VendorOwnerID string `json:"vendor_owner_id"`
}

type VendorsList struct {
	Page    Page      `json:"page"`
	Vendors []*Vendor `json:"data"`
}

type VendorsResponse struct {
	Vendors    []*Vendor
	Pagination string
}
