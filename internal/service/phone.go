package service

import (
	"strings"

	bizerrors "polaris/internal/errors"
)

// normalizePhone converts common phone-number spellings to one rate-limit key.
// Mainland numbers are normalized to E.164 (+86...) so 138..., 86138..., 0086...
// and +86... cannot be used to obtain separate SMS quotas for the same handset.
func normalizePhone(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", bizerrors.New(bizerrors.CodeInvalidParams, "phone number is required")
	}

	var digits strings.Builder
	hasPlus := false
	for i, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '+' && i == 0:
			hasPlus = true
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
			continue
		default:
			return "", bizerrors.New(bizerrors.CodeInvalidParams, "invalid phone number")
		}
	}

	number := digits.String()
	if strings.HasPrefix(number, "00") {
		number = number[2:]
		hasPlus = true
	}
	if len(number) < 6 || len(number) > 15 {
		return "", bizerrors.New(bizerrors.CodeInvalidParams, "invalid phone number")
	}

	// Mainland China: normalize both the national 11-digit form and the
	// country-code-prefixed form to +86<11 digits>.
	if !hasPlus && len(number) == 11 && number[0] == '1' {
		return "+86" + number, nil
	}
	if len(number) == 13 && strings.HasPrefix(number, "86") && number[2] == '1' {
		return "+" + number, nil
	}
	if hasPlus {
		return "+" + number, nil
	}
	return number, nil
}

// phoneLookupCandidates returns the common storage forms for a normalized phone.
// Existing rows may predate normalization, so lookups must not assume one spelling.
func phoneLookupCandidates(canonical string) []string {
	if canonical == "" {
		return nil
	}
	out := make([]string, 0, 4)
	seen := map[string]struct{}{}
	add := func(value string) {
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	add(canonical)
	add(strings.TrimPrefix(canonical, "+"))
	if strings.HasPrefix(canonical, "+86") && len(canonical) == 14 {
		national := canonical[3:]
		add(national)
		add("86" + national)
	}
	if strings.HasPrefix(canonical, "86") && len(canonical) == 13 {
		national := canonical[2:]
		add(national)
		add("+" + canonical)
	}
	return out
}
