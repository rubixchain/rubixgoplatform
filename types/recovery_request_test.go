package types

import "testing"

func TestRecoveryRequestSigningBytes(t *testing.T) {
	request := RecoveryRequest{
		DID:     "user-did",
		Network: "testnet",
		Purpose: RecoveryRequestPurpose,
		Epoch:   1794700800,
	}
	want := `{"did":"user-did","network":"testnet","purpose":"account recovery","epoch":1794700800}`
	for _, signature := range []string{"", "user-signature"} {
		request.Signature = signature
		got, err := request.SigningBytes()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("signing bytes = %s, want %s", got, want)
		}
	}
}
