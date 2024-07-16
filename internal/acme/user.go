package acme

import (
	"crypto"
	"github.com/go-acme/lego/v4/registration"
)

type RegistrationUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *RegistrationUser) GetEmail() string {
	return u.Email
}

func (u RegistrationUser) GetRegistration() *registration.Resource {
	return u.Registration
}

func (u *RegistrationUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}
