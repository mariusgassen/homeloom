// Package domain holds Homeloom's entities and business errors. It imports nothing from adapters.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Identity is what an identity provider tells us about the caller.
type Identity struct {
	Subject   string
	Name      string
	Email     string
	AvatarURL string
}

type User struct {
	ID        uuid.UUID
	Name      string
	Email     string
	AvatarURL string
}

type Role string

const (
	RoleOwner  Role = "owner"
	RoleMember Role = "member"
)

// MaxHouseholdName is the longest household name, in characters.
const MaxHouseholdName = 100

type Household struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}

// Membership is a household as seen by one of its members.
type Membership struct {
	Household Household
	Role      Role
}

type Member struct {
	User User
	Role Role
}

type HouseholdDetail struct {
	Membership
	Members []Member
}
