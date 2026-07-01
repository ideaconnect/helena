package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/idct/helena/internal/model"
)

// NTLM (NTLMv2) message + crypto primitives (#78). NTLM is challenge/response
// over a single connection: NEGOTIATE (type 1) → CHALLENGE (type 2) →
// AUTHENTICATE (type 3). This file is the wire + crypto core; the handshake is
// driven by internal/httpclient. MD4 (the NT hash's inner hash) is not in the
// Go standard library, so it is implemented here from RFC 1320 — keeping the
// package free of third-party dependencies.

const ntlmSignature = "NTLMSSP\x00"

// NTLM negotiate flags (MS-NLMP §2.2.2.5), only the subset we set/inspect.
const (
	ntlmNegotiateUnicode          = 0x00000001
	ntlmRequestTarget             = 0x00000004
	ntlmNegotiateNTLM             = 0x00000200
	ntlmNegotiateAlwaysSign       = 0x00008000
	ntlmNegotiateExtendedSecurity = 0x00080000
	ntlmNegotiateTargetInfo       = 0x00800000
)

// le is the byte order for every NTLM field.
var le = binary.LittleEndian

// md4 implements RFC 1320. Returns the 16-byte digest. NTLM uses it once, on the
// UTF-16LE password, to derive the NT hash.
func md4(data []byte) [16]byte {
	var a, b, c, d uint32 = 0x67452301, 0xefcdab89, 0x98badcfe, 0x10325476

	ml := uint64(len(data)) * 8
	data = append(append([]byte(nil), data...), 0x80)
	for len(data)%64 != 56 {
		data = append(data, 0)
	}
	var lenbuf [8]byte
	le.PutUint64(lenbuf[:], ml)
	data = append(data, lenbuf[:]...)

	f := func(x, y, z uint32) uint32 { return (x & y) | (^x & z) }
	g := func(x, y, z uint32) uint32 { return (x & y) | (x & z) | (y & z) }
	h := func(x, y, z uint32) uint32 { return x ^ y ^ z }
	rotl := func(x uint32, n uint) uint32 { return (x << n) | (x >> (32 - n)) }

	for off := 0; off < len(data); off += 64 {
		var x [16]uint32
		for i := 0; i < 16; i++ {
			x[i] = le.Uint32(data[off+i*4:])
		}
		aa, bb, cc, dd := a, b, c, d
		for _, i := range []int{0, 4, 8, 12} {
			aa = rotl(aa+f(bb, cc, dd)+x[i], 3)
			dd = rotl(dd+f(aa, bb, cc)+x[i+1], 7)
			cc = rotl(cc+f(dd, aa, bb)+x[i+2], 11)
			bb = rotl(bb+f(cc, dd, aa)+x[i+3], 19)
		}
		for _, i := range []int{0, 1, 2, 3} {
			aa = rotl(aa+g(bb, cc, dd)+x[i]+0x5a827999, 3)
			dd = rotl(dd+g(aa, bb, cc)+x[i+4]+0x5a827999, 5)
			cc = rotl(cc+g(dd, aa, bb)+x[i+8]+0x5a827999, 9)
			bb = rotl(bb+g(cc, dd, aa)+x[i+12]+0x5a827999, 13)
		}
		for _, i := range []int{0, 2, 1, 3} {
			aa = rotl(aa+h(bb, cc, dd)+x[i]+0x6ed9eba1, 3)
			dd = rotl(dd+h(aa, bb, cc)+x[i+8]+0x6ed9eba1, 9)
			cc = rotl(cc+h(dd, aa, bb)+x[i+4]+0x6ed9eba1, 11)
			bb = rotl(bb+h(cc, dd, aa)+x[i+12]+0x6ed9eba1, 15)
		}
		a, b, c, d = a+aa, b+bb, c+cc, d+dd
	}
	var out [16]byte
	le.PutUint32(out[0:], a)
	le.PutUint32(out[4:], b)
	le.PutUint32(out[8:], c)
	le.PutUint32(out[12:], d)
	return out
}

// utf16le encodes s as little-endian UTF-16 (the on-the-wire form for every
// NTLM string field when Unicode is negotiated).
func utf16le(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		le.PutUint16(b[i*2:], c)
	}
	return b
}

// ntowfv2 derives the NTLMv2 response key (MS-NLMP §3.3.2):
// HMAC_MD5(MD4(UTF16LE(password)), UTF16LE(Uppercase(user) + domain)).
func ntowfv2(password, user, domain string) []byte {
	nt := md4(utf16le(password))
	mac := hmac.New(md5.New, nt[:])
	mac.Write(utf16le(strings.ToUpper(user) + domain))
	return mac.Sum(nil)
}

// ntlmv2Response computes the NtChallengeResponse and the NTProofStr
// (MS-NLMP §3.3.2). targetInfo is the AV_PAIR blob copied from the server's
// CHALLENGE; timestamp and clientChallenge are 8 bytes each.
func ntlmv2Response(ntowf, serverChallenge, clientChallenge, timestamp, targetInfo []byte) (ntResp, ntProof []byte) {
	var temp bytes.Buffer
	temp.Write([]byte{0x01, 0x01, 0, 0, 0, 0, 0, 0}) // RespType, HiRespType, reserved
	temp.Write(timestamp)                            // 8 bytes
	temp.Write(clientChallenge)                      // 8 bytes
	temp.Write([]byte{0, 0, 0, 0})                   // reserved
	temp.Write(targetInfo)
	temp.Write([]byte{0, 0, 0, 0}) // reserved

	mac := hmac.New(md5.New, ntowf)
	mac.Write(serverChallenge)
	mac.Write(temp.Bytes())
	ntProof = mac.Sum(nil)

	ntResp = append(append([]byte(nil), ntProof...), temp.Bytes()...)
	return ntResp, ntProof
}

// lmv2Response computes the LMv2 response (MS-NLMP §3.3.2):
// HMAC_MD5(ntowf, serverChallenge || clientChallenge) || clientChallenge.
func lmv2Response(ntowf, serverChallenge, clientChallenge []byte) []byte {
	mac := hmac.New(md5.New, ntowf)
	mac.Write(serverChallenge)
	mac.Write(clientChallenge)
	return append(mac.Sum(nil), clientChallenge...)
}

// ntlmNegotiate builds the type-1 NEGOTIATE message. Domain and workstation are
// left empty (the server supplies the target via the CHALLENGE).
func ntlmNegotiate() []byte {
	var b bytes.Buffer
	b.WriteString(ntlmSignature)
	_ = binary.Write(&b, le, uint32(1)) // MessageType
	_ = binary.Write(&b, le, uint32(ntlmNegotiateUnicode|ntlmRequestTarget|ntlmNegotiateNTLM|ntlmNegotiateAlwaysSign|ntlmNegotiateExtendedSecurity))
	// DomainNameFields + WorkstationFields: empty (len 0, offset = header size 32).
	_ = binary.Write(&b, le, uint32(0)) // domain len+maxlen
	_ = binary.Write(&b, le, uint32(32))
	_ = binary.Write(&b, le, uint32(0)) // workstation len+maxlen
	_ = binary.Write(&b, le, uint32(32))
	return b.Bytes()
}

// errBadChallenge is returned when a CHALLENGE message can't be parsed.
var errBadChallenge = errors.New("ntlm: malformed CHALLENGE message")

// parseChallenge extracts the server challenge, target-info blob, and negotiate
// flags from a type-2 CHALLENGE message (MS-NLMP §2.2.1.2).
func parseChallenge(msg []byte) (serverChallenge, targetInfo []byte, flags uint32, err error) {
	if len(msg) < 32 || string(msg[:8]) != ntlmSignature || le.Uint32(msg[8:]) != 2 {
		return nil, nil, 0, errBadChallenge
	}
	flags = le.Uint32(msg[20:])
	serverChallenge = append([]byte(nil), msg[24:32]...)
	// TargetInfoFields live at offset 40 (after the 8-byte TargetNameFields at 12
	// and the reserved 8 bytes at 32). Present only when the message is long
	// enough and TARGET_INFO was negotiated.
	if len(msg) >= 48 {
		tiLen := int(le.Uint16(msg[40:]))
		tiOff := int(le.Uint32(msg[44:]))
		if tiLen > 0 && tiOff+tiLen <= len(msg) {
			targetInfo = append([]byte(nil), msg[tiOff:tiOff+tiLen]...)
		}
	}
	return serverChallenge, targetInfo, flags, nil
}

// ntlmAuthenticate assembles a type-3 AUTHENTICATE message (MS-NLMP §2.2.1.3)
// from the already-computed LM/NT responses and the string fields. No version
// block or MIC is emitted (sufficient for NTLMv2 HTTP auth).
func ntlmAuthenticate(domain, user, workstation string, lmResp, ntResp []byte, flags uint32) []byte {
	dom := utf16le(domain)
	usr := utf16le(user)
	ws := utf16le(workstation)

	const headerLen = 64 // sig(8)+type(4)+6 buffers(48)+flags(4)
	var payload bytes.Buffer
	field := func(data []byte) (lenOff [8]byte) {
		off := headerLen + payload.Len()
		payload.Write(data)
		le.PutUint16(lenOff[0:], uint16(len(data)))
		le.PutUint16(lenOff[2:], uint16(len(data)))
		le.PutUint32(lenOff[4:], uint32(off))
		return lenOff
	}
	// Payload order: LmResponse, NtResponse, Domain, User, Workstation, SessionKey.
	lmF := field(lmResp)
	ntF := field(ntResp)
	domF := field(dom)
	usrF := field(usr)
	wsF := field(ws)
	skF := field(nil) // no exported session key

	var b bytes.Buffer
	b.WriteString(ntlmSignature)
	_ = binary.Write(&b, le, uint32(3)) // MessageType
	b.Write(lmF[:])
	b.Write(ntF[:])
	b.Write(domF[:])
	b.Write(usrF[:])
	b.Write(wsF[:])
	b.Write(skF[:])
	_ = binary.Write(&b, le, flags)
	b.Write(payload.Bytes())
	return b.Bytes()
}

// NTLMOffered reports whether a 401 response's WWW-Authenticate headers invite
// an NTLM handshake (#78).
func NTLMOffered(wwwAuthenticate []string) bool {
	for _, h := range wwwAuthenticate {
		f := strings.Fields(h)
		if len(f) > 0 && strings.EqualFold(f[0], "NTLM") {
			return true
		}
	}
	return false
}

// NTLMNegotiateHeader returns the Authorization header value carrying the
// type-1 NEGOTIATE message (#78). The client sends this after the initial 401.
func NTLMNegotiateHeader() string {
	return "NTLM " + base64.StdEncoding.EncodeToString(ntlmNegotiate())
}

// NTLMChallenge extracts the base64-decoded type-2 CHALLENGE blob from a 401
// response's WWW-Authenticate headers (the `NTLM <base64>` form). ok is false
// when no challenge token is present.
func NTLMChallenge(wwwAuthenticate []string) (challenge []byte, ok bool) {
	for _, h := range wwwAuthenticate {
		f := strings.Fields(h)
		if len(f) == 2 && strings.EqualFold(f[0], "NTLM") {
			if b, err := base64.StdEncoding.DecodeString(f[1]); err == nil {
				return b, true
			}
		}
	}
	return nil, false
}

// NTLMAuthenticateHeader builds the Authorization header carrying the type-3
// AUTHENTICATE message computed from the server CHALLENGE and the credentials
// (#78). A fresh random client challenge and the current time are used.
func NTLMAuthenticateHeader(challenge []byte, a model.NTLMAuth) (string, error) {
	cc := make([]byte, 8)
	if _, err := rand.Read(cc); err != nil {
		return "", fmt.Errorf("ntlm: client challenge: %w", err)
	}
	msg, err := ntlmType3(a.Username, a.Password, a.Domain, a.Workstation, challenge, cc, ntlmTimestamp())
	if err != nil {
		return "", err
	}
	return "NTLM " + base64.StdEncoding.EncodeToString(msg), nil
}

// ntlmTimestamp returns the current time as a Windows FILETIME (100ns ticks
// since 1601-01-01), little-endian, 8 bytes — the form NTLMv2 embeds.
func ntlmTimestamp() []byte {
	const epochDelta = 116444736000000000 // 100ns ticks between 1601 and 1970
	ticks := uint64(time.Now().UnixNano()/100) + epochDelta
	b := make([]byte, 8)
	le.PutUint64(b, ticks)
	return b
}

// ntlmType3 ties the handshake together: given the server's CHALLENGE and the
// credentials, it computes the NTLMv2 responses and returns the AUTHENTICATE
// message. clientChallenge (8 bytes) and timestamp (8 bytes, Windows FILETIME)
// are passed in so the result is deterministic for tests; production callers
// generate a random nonce + current time.
func ntlmType3(user, password, domain, workstation string, challenge, clientChallenge, timestamp []byte) ([]byte, error) {
	serverChallenge, targetInfo, flags, err := parseChallenge(challenge)
	if err != nil {
		return nil, err
	}
	ntowf := ntowfv2(password, user, domain)
	ntResp, _ := ntlmv2Response(ntowf, serverChallenge, clientChallenge, timestamp, targetInfo)
	lmResp := lmv2Response(ntowf, serverChallenge, clientChallenge)
	// Echo the unicode + NTLM flag set so the server interprets the response as
	// Unicode NTLMv2; preserve the server's extended-security bits.
	respFlags := flags&(ntlmNegotiateUnicode|ntlmNegotiateExtendedSecurity|ntlmNegotiateTargetInfo) | ntlmNegotiateNTLM | ntlmNegotiateAlwaysSign
	return ntlmAuthenticate(domain, user, workstation, lmResp, ntResp, respFlags), nil
}
