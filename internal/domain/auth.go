package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

var (
	ErrAuthenticationUnavailable = errors.New("authentication unavailable")
	ErrDiscordUnavailable        = errors.New("discord unavailable")
	ErrInvalidSession            = errors.New("invalid session")
	ErrUserDisabled              = errors.New("user disabled")
)

type User struct {
	ID              int64
	DiscordID       string
	DiscordUsername string
	DisplayName     string
	AvatarURL       *string
	Handle          string
	Location        *string
	Bio             *string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type DiscordIdentity struct {
	ID          string
	Username    string
	DisplayName string
	AvatarURL   *string
}

func (i DiscordIdentity) Validate() error {
	if i.ID == "" {
		return fmt.Errorf("discord identity ID is empty")
	}
	for _, character := range i.ID {
		if character < '0' || character > '9' {
			return fmt.Errorf("discord identity ID is not numeric")
		}
	}
	if strings.TrimSpace(i.Username) == "" {
		return fmt.Errorf("discord identity username is empty")
	}
	if len([]rune(i.Username)) > 100 {
		return fmt.Errorf("discord identity username is too long")
	}
	if strings.TrimSpace(i.DisplayName) == "" {
		return fmt.Errorf("discord identity display name is empty")
	}
	if len([]rune(i.DisplayName)) > 100 {
		return fmt.Errorf("discord identity display name is too long")
	}
	return nil
}

func NormalizeHandle(username string) string {
	var normalized strings.Builder
	previousSeparator := false
	for _, character := range strings.ToLower(username) {
		isLetter := character >= 'a' && character <= 'z'
		isDigit := character >= '0' && character <= '9'
		if isLetter || isDigit {
			normalized.WriteRune(character)
			previousSeparator = false
			continue
		}
		if normalized.Len() > 0 && !previousSeparator {
			normalized.WriteByte('-')
			previousSeparator = true
		}
	}

	base := strings.Trim(normalized.String(), "-")
	if len(base) < 3 {
		if base == "" {
			base = "user"
		} else {
			base = "user-" + base
		}
	}
	if len(base) > 40 {
		base = strings.TrimRight(base[:40], "-")
	}
	return base
}

func HandleCandidate(base string, attempt int) string {
	if attempt <= 1 {
		return base
	}
	suffix := fmt.Sprintf("-%d", attempt)
	maximumBaseLength := 40 - len(suffix)
	trimmedBase := strings.TrimRight(base[:min(len(base), maximumBaseLength)], "-")
	return trimmedBase + suffix
}
