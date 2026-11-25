package users

import (
	"net/mail"
	"strings"

	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
)

type CreateUserRequest struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

func (r *CreateUserRequest) Validate() error {
	var errs []string
	if strings.TrimSpace(r.Email) == "" {
		errs = append(errs, "email should not be empty")
	} else if !isValidEmail(r.Email) {
		errs = append(errs, "email must be an email")
	}
	if strings.TrimSpace(r.Name) == "" {
		errs = append(errs, "name should not be empty")
	}
	if strings.TrimSpace(r.Address) == "" {
		errs = append(errs, "address should not be empty")
	}
	if len(errs) > 0 {
		return httputil.BadRequestMessages(errs)
	}
	return nil
}

type UpdateUserRequest struct {
	Email   *string `json:"email"`
	Name    *string `json:"name"`
	Address *string `json:"address"`
}

func (r *UpdateUserRequest) Validate() error {
	var errs []string
	if r.Email != nil && !isValidEmail(*r.Email) {
		errs = append(errs, "email must be an email")
	}
	if r.Name != nil && strings.TrimSpace(*r.Name) == "" {
		errs = append(errs, "name should not be empty")
	}
	if r.Address != nil && strings.TrimSpace(*r.Address) == "" {
		errs = append(errs, "address should not be empty")
	}
	if len(errs) > 0 {
		return httputil.BadRequestMessages(errs)
	}
	return nil
}

type UserQuery struct {
	Skip   *int
	Take   *int
	Search *string
}

func (q *UserQuery) Validate() error {
	var errs []string
	if q.Skip != nil && *q.Skip < 0 {
		errs = append(errs, "skip must not be less than 0")
	}
	if q.Take != nil {
		if *q.Take < 1 {
			errs = append(errs, "take must be a positive number")
		} else if *q.Take > 100 {
			errs = append(errs, "take must not be greater than 100")
		}
	}
	if len(errs) > 0 {
		return httputil.BadRequestMessages(errs)
	}
	return nil
}

func isValidEmail(raw string) bool {
	if strings.ContainsAny(raw, " \t\n\r<>") {
		return false
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Address != raw {
		return false
	}
	at := strings.IndexByte(raw, '@')
	if at <= 0 || at == len(raw)-1 {
		return false
	}
	domainPart := raw[at+1:]
	return strings.Contains(domainPart, ".") && !strings.HasPrefix(domainPart, ".") && !strings.HasSuffix(domainPart, ".")
}
