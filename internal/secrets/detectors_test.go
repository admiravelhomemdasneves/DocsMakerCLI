package secrets

import (
	"strings"
	"testing"
)

func TestDefaultScanner_Detects(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		kind  Kind
		match string
	}{
		{"jdbc url with credentials", `String u = "jdbc:mysql://admin:hunter2@localhost:3306/sampledb";`, KindConnString, "jdbc:mysql://admin:hunter2@localhost:3306/sampledb"},
		{"mongodb url with credentials", `uri = "mongodb+srv://app:s3cr3tPw@cluster0.example.net/db"`, KindConnString, "mongodb+srv://app:s3cr3tPw@cluster0.example.net/db"},
		{"jdbc password parameter", `"jdbc:sqlserver://host:1433;databaseName=x;password=Abc12345"`, KindConnString, "jdbc:sqlserver://host:1433;databaseName=x;password=Abc12345"},
		{"aws access key", `String k = "AKIAIOSFODNN7EXAMPLE";`, KindAWSKey, "AKIAIOSFODNN7EXAMPLE"},
		{"jwt", `t = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"`, KindJWT, "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"},
		{"pem private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA\n-----END RSA PRIVATE KEY-----", KindPrivateKey, "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA\n-----END RSA PRIVATE KEY-----"},
		{"github token", `String t = "ghp_abcdefghijklmnopqrstuvwxyz0123456789";`, KindAPIToken, "ghp_abcdefghijklmnopqrstuvwxyz0123456789"},
		{"password assignment", `private String dbPassword = "hunter2";`, KindGenericSecret, "hunter2"},
		{"json api key", `{"apiKey": "abc123def456"}`, KindGenericSecret, "abc123def456"},
		{"bearer token", `Authorization: Bearer abcdefghijklmnopqrstuvwxyz012345`, KindGenericSecret, "abcdefghijklmnopqrstuvwxyz012345"},
		{"opaque random literal", `String s = "q8Zr3kV9xLp2Wm7Nc4Yb6Ht1Ds5Fg0Jq";`, KindHighEntropy, "q8Zr3kV9xLp2Wm7Nc4Yb6Ht1Ds5Fg0Jq"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DefaultScanner{}.Scan(tc.text)
			if len(got) != 1 {
				t.Fatalf("got %d findings %+v, want exactly 1", len(got), got)
			}
			if got[0].Kind != tc.kind || got[0].Match != tc.match {
				t.Fatalf("got {%s %q}, want {%s %q}", got[0].Kind, got[0].Match, tc.kind, tc.match)
			}
			if tc.text[got[0].StartByte:got[0].EndByte] != got[0].Match {
				t.Fatalf("offsets [%d,%d) don't select Match", got[0].StartByte, got[0].EndByte)
			}
		})
	}
}

func TestDefaultScanner_IgnoresBenignCode(t *testing.T) {
	benign := []string{
		`String tokenType = "Bearer";`,
		`String passwordPolicy = "strict-rotation";`,
		`private String password = "${DB_PASSWORD}";`,
		`String password = "changeme";`,
		`String dbPassword = "DB_PASSWORD";`,
		`String pw = System.getenv("DB_PASSWORD");`,
		`String id = "123e4567-e89b-12d3-a456-426614174000";`,
		`String c = "com.example.animal.Animal";`,
		`String url = "https://example.com/api/v1/users/list";`,
		`String name = "docsmaker-sample-app-version-1";`,
		`String greeting = "Hello there, this is a perfectly normal sentence";`,
		`public String getName() { return name; }`,
		`if (password == "x") {}`,
	}
	for _, text := range benign {
		if got := (DefaultScanner{}).Scan(text); len(got) != 0 {
			t.Errorf("Scan(%q) = %+v, want no findings", text, got)
		}
	}
}

func TestDefaultScanner_SpecificRuleWinsOverGeneric(t *testing.T) {
	text := `String password = "AKIAIOSFODNN7EXAMPLE";`
	got := DefaultScanner{}.Scan(text)
	if len(got) != 1 || got[0].Kind != KindAWSKey {
		t.Fatalf("got %+v, want a single aws-key finding", got)
	}
}

func TestRedact(t *testing.T) {
	text := `a = "jdbc:mysql://admin:hunter2@localhost/db"; b = "q8Zr3kV9xLp2Wm7Nc4Yb6Ht1Ds5Fg0Jq";`
	findings := DefaultScanner{}.Scan(text)
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(findings), findings)
	}

	got := Redact(text, findings)
	for _, secret := range []string{"hunter2", "q8Zr3kV9"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted text still contains %q: %s", secret, got)
		}
	}
	if !strings.Contains(got, "[REDACTED:connection-string]") || !strings.Contains(got, "[REDACTED:high-entropy]") {
		t.Fatalf("missing placeholders: %s", got)
	}
}

func TestMask_NeverRevealsWholeSecret(t *testing.T) {
	secret := "jdbc:mysql://admin:hunter2@localhost:3306/sampledb"
	if m := Mask(secret); strings.Contains(m, "hunter2") {
		t.Fatalf("Mask leaked the password: %s", m)
	}
	if m := Mask("abc"); m != "a***" {
		t.Fatalf("Mask(short) = %q, want a***", m)
	}
}
