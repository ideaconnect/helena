package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestNTLMResolveAndApply verifies {{vars}} substitution on NTLM credentials and
// that Apply is a no-op (the handshake is the httpclient's job).
func TestNTLMResolveAndApply(t *testing.T) {
	a := ResolveValues(model.Auth{Type: model.AuthNTLM, NTLM: &model.NTLMAuth{
		Username: "{{u}}", Password: "p", Domain: "{{d}}",
	}}, func(s string) string {
		switch s {
		case "{{u}}":
			return "alice"
		case "{{d}}":
			return "CORP"
		}
		return s
	})
	if a.NTLM.Username != "alice" || a.NTLM.Domain != "CORP" {
		t.Errorf("resolve = %+v", a.NTLM)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://x/", nil)
	if err := Apply(context.Background(), req, a, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Error("NTLM Apply must not set Authorization (handshake is in httpclient)")
	}
}

// TestMD4RFC1320 pins the hand-rolled MD4 against the RFC 1320 §A.5 test suite.
func TestMD4RFC1320(t *testing.T) {
	cases := map[string]string{
		"":                           "31d6cfe0d16ae931b73c59d7e0c089c0",
		"a":                          "bde52cb31de33e46245e05fbdbd6fb24",
		"abc":                        "a448017aaf21d8525fc10ae87aa6729d",
		"message digest":             "d9130a8164549fe818874806e1c7014b",
		"abcdefghijklmnopqrstuvwxyz": "d79e1c308aa5bbcdeea8ed63df412da9",
	}
	for in, want := range cases {
		got := md4([]byte(in))
		if hex.EncodeToString(got[:]) != want {
			t.Errorf("md4(%q) = %x, want %s", in, got, want)
		}
	}
}

// msNLMP holds the MS-NLMP §4.2.4 worked-example fixture values.
var (
	msServerChallenge = mustHex("0123456789abcdef")
	msClientChallenge = bytes.Repeat([]byte{0xaa}, 8)
	msTimestamp       = make([]byte, 8)
	msTargetInfo      = mustHex("02000c0044006f006d00610069006e00" + "01000c00530065007200760065007200" + "00000000")
)

// TestNTOWFv2 pins the NTLMv2 key derivation against MS-NLMP §4.2.4.1.1.
func TestNTOWFv2(t *testing.T) {
	got := ntowfv2("Password", "User", "Domain")
	if hex.EncodeToString(got) != "0c868a403bfd7a93a3001ef22ef02e3f" {
		t.Errorf("ntowfv2 = %x", got)
	}
}

// TestNTLMv2ResponseAndLMv2 pins the NTProofStr and LMv2 response against
// MS-NLMP §4.2.4.2.1 / §4.2.4.2.2.
func TestNTLMv2ResponseAndLMv2(t *testing.T) {
	ntowf := ntowfv2("Password", "User", "Domain")
	ntResp, ntProof := ntlmv2Response(ntowf, msServerChallenge, msClientChallenge, msTimestamp, msTargetInfo)
	if hex.EncodeToString(ntProof) != "68cd0ab851e51c96aabc927bebef6a1c" {
		t.Errorf("NTProofStr = %x", ntProof)
	}
	// NtChallengeResponse = NTProofStr || temp.
	if !bytes.Equal(ntResp[:16], ntProof) {
		t.Error("NtChallengeResponse must start with the NTProofStr")
	}
	lm := lmv2Response(ntowf, msServerChallenge, msClientChallenge)
	if hex.EncodeToString(lm) != "86c35097ac9cec102554764a57cccc19aaaaaaaaaaaaaaaa" {
		t.Errorf("LMv2 = %x", lm)
	}
}

// TestNTLMNegotiate verifies the type-1 message header (signature + type + a
// couple of expected flags).
func TestNTLMNegotiate(t *testing.T) {
	m := ntlmNegotiate()
	if string(m[:8]) != ntlmSignature || le.Uint32(m[8:]) != 1 {
		t.Fatalf("bad NEGOTIATE header: %x", m[:12])
	}
	flags := le.Uint32(m[12:])
	if flags&ntlmNegotiateUnicode == 0 || flags&ntlmNegotiateNTLM == 0 {
		t.Errorf("negotiate flags missing unicode/ntlm: %08x", flags)
	}
}

// TestParseChallengeRoundTrip builds a synthetic CHALLENGE and parses it back.
func TestParseChallengeRoundTrip(t *testing.T) {
	msg := buildChallenge(msServerChallenge, msTargetInfo)
	sc, ti, flags, err := parseChallenge(msg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sc, msServerChallenge) {
		t.Errorf("serverChallenge = %x", sc)
	}
	if !bytes.Equal(ti, msTargetInfo) {
		t.Errorf("targetInfo = %x", ti)
	}
	if flags&ntlmNegotiateUnicode == 0 {
		t.Errorf("flags = %08x", flags)
	}
	// A truncated / mis-signed message is rejected.
	if _, _, _, err := parseChallenge([]byte("not-ntlm")); err == nil {
		t.Error("expected error on malformed CHALLENGE")
	}
}

// TestNTLMType3 drives the full handshake response: feed a CHALLENGE, build the
// AUTHENTICATE, and verify its NtChallengeResponse embeds the known NTProofStr
// and its User/Domain fields decode to the inputs.
func TestNTLMType3(t *testing.T) {
	challenge := buildChallenge(msServerChallenge, msTargetInfo)
	msg, err := ntlmType3("User", "Password", "Domain", "WS", challenge, msClientChallenge, msTimestamp)
	if err != nil {
		t.Fatal(err)
	}
	if string(msg[:8]) != ntlmSignature || le.Uint32(msg[8:]) != 3 {
		t.Fatalf("bad AUTHENTICATE header: %x", msg[:12])
	}
	nt := readField(msg, 20)  // NtChallengeResponse security buffer at offset 20
	dom := readField(msg, 28) // DomainName at 28
	usr := readField(msg, 36) // UserName at 36
	if !bytes.Equal(nt[:16], mustHex("68cd0ab851e51c96aabc927bebef6a1c")) {
		t.Errorf("AUTHENTICATE NtChallengeResponse proof = %x", nt[:16])
	}
	if !bytes.Equal(dom, utf16le("Domain")) || !bytes.Equal(usr, utf16le("User")) {
		t.Errorf("domain/user fields wrong: dom=%x usr=%x", dom, usr)
	}

	// A malformed CHALLENGE surfaces as an error rather than a bogus message.
	if _, err := ntlmType3("User", "Password", "Domain", "WS", []byte("bad"), msClientChallenge, msTimestamp); err == nil {
		t.Error("expected error for malformed challenge")
	}
}

// TestNTLMExportedHelpers covers the handshake API the httpclient calls:
// offer detection, the type-1 header, challenge extraction, and the type-3
// header (decoded back to verify it is a valid AUTHENTICATE for the creds).
func TestNTLMExportedHelpers(t *testing.T) {
	if !NTLMOffered([]string{"Basic realm=x", "NTLM"}) {
		t.Error("NTLMOffered should detect a bare NTLM offer")
	}
	if NTLMOffered([]string{"Negotiate", "Basic"}) {
		t.Error("NTLMOffered should not match non-NTLM schemes")
	}

	neg := NTLMNegotiateHeader()
	if !strings.HasPrefix(neg, "NTLM ") {
		t.Fatalf("negotiate header = %q", neg)
	}
	t1, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(neg, "NTLM "))
	if string(t1[:8]) != ntlmSignature || le.Uint32(t1[8:]) != 1 {
		t.Errorf("type-1 message malformed: %x", t1[:12])
	}

	chMsg := buildChallenge(msServerChallenge, msTargetInfo)
	b64 := base64.StdEncoding.EncodeToString(chMsg)
	challenge, ok := NTLMChallenge([]string{"NTLM " + b64})
	if !ok || !bytes.Equal(challenge, chMsg) {
		t.Fatalf("NTLMChallenge ok=%v", ok)
	}
	if _, ok := NTLMChallenge([]string{"NTLM"}); ok {
		t.Error("a bare NTLM (no token) is not a challenge")
	}

	hdr, err := NTLMAuthenticateHeader(challenge, model.NTLMAuth{Username: "User", Password: "Password", Domain: "Domain"})
	if err != nil || !strings.HasPrefix(hdr, "NTLM ") {
		t.Fatalf("authenticate header = %q err=%v", hdr, err)
	}
	t3, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(hdr, "NTLM "))
	if string(t3[:8]) != ntlmSignature || le.Uint32(t3[8:]) != 3 {
		t.Errorf("type-3 message malformed: %x", t3[:12])
	}
	if usr := readField(t3, 36); !bytes.Equal(usr, utf16le("User")) {
		t.Errorf("type-3 user field = %x", usr)
	}
}

// buildChallenge assembles a minimal type-2 CHALLENGE message (no version block).
func buildChallenge(serverChallenge, targetInfo []byte) []byte {
	const headerLen = 48
	var b bytes.Buffer
	b.WriteString(ntlmSignature)
	_ = binary.Write(&b, le, uint32(2)) // type 2
	// TargetNameFields (empty) at 12.
	_ = binary.Write(&b, le, uint32(0))
	_ = binary.Write(&b, le, uint32(headerLen))
	_ = binary.Write(&b, le, uint32(ntlmNegotiateUnicode|ntlmNegotiateTargetInfo)) // flags at 20
	b.Write(serverChallenge)                                                       // 24..32
	b.Write(make([]byte, 8))                                                       // reserved 32..40
	// TargetInfoFields at 40.
	_ = binary.Write(&b, le, uint16(len(targetInfo)))
	_ = binary.Write(&b, le, uint16(len(targetInfo)))
	_ = binary.Write(&b, le, uint32(headerLen))
	b.Write(targetInfo)
	return b.Bytes()
}

// readField slices a security-buffer payload (len@off, offset@off+4) out of msg.
func readField(msg []byte, off int) []byte {
	n := int(le.Uint16(msg[off:]))
	start := int(le.Uint32(msg[off+4:]))
	return msg[start : start+n]
}

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}
