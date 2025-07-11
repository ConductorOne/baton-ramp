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

type Role struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
