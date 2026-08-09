package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaximumProfileLocationLength = 100
	MaximumProfileBioLength      = 500
)

// SellerProfile is the public projection of a seller and their visible inventory.
type SellerProfile struct {
	User               PublicUser
	CreatedAt          time.Time
	ActiveListingCount int64
}

// NormalizeProfileLocation trims a location, treating an empty value as absent.
func NormalizeProfileLocation(value *string) (*string, error) {
	return normalizeOptionalProfileValue(value, "location", MaximumProfileLocationLength)
}

// NormalizeProfileBio trims a bio, treating an empty value as absent.
func NormalizeProfileBio(value *string) (*string, error) {
	return normalizeOptionalProfileValue(value, "bio", MaximumProfileBioLength)
}

func normalizeOptionalProfileValue(value *string, field string, maximum int) (*string, error) {
	if value == nil {
		return nil, nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(normalized) > maximum {
		return nil, NewValidationError(map[string]string{field: fmt.Sprintf("must be at most %d characters", maximum)})
	}
	return &normalized, nil
}

// IsCanonicalHandle reports whether value matches the persisted public-handle grammar.
func IsCanonicalHandle(value string) bool {
	if len(value) < 3 || len(value) > 40 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
			continue
		}
		return false
	}
	return true
}
