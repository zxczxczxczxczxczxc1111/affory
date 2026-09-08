package skorost

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestProvidersOnAuthorizedGuest(t *testing.T) {
	if os.Getenv("AFFORY_TEST_SPEED_NETWORK") != "guest-authorized" {
		t.Skip("explicit guest network test only")
	}
	client, err := Client("")
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	for _, provider := range Default().Providers {
		t.Run(provider.ID, func(t *testing.T) {
			runner := Default()
			runner.Providers = []Provider{provider}
			result := runner.Run(context.Background(), client, "", nil)
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			t.Log(string(encoded))
			if result.Error != "" {
				t.Error("provider unavailable on this guest connection")
			}
		})
	}
}
