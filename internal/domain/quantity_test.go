package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func TestQuantityJSONPreservesThousandths(t *testing.T) {
	var q domain.Quantity
	if err := json.Unmarshal([]byte(`"9223372036854775.807"`), &q); err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(q)
	if err != nil || string(got) != `"9223372036854775.807"` {
		t.Fatalf("quantity = %s, %v", got, err)
	}
	for _, bad := range []string{`"9223372036854775.808"`, `"1.0001"`, `"-1"`, `1.25`, `null`, `"1e3"`} {
		if err := json.Unmarshal([]byte(bad), &q); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}
