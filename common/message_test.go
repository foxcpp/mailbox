package common

import (
	"fmt"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func prettyPrint(msg *Msg) string {
	res := fmt.Sprintf(`[%v] %v
To: %v
From: %v  ReplyTo: %v
Cc: %v  Bcc: %v
Other headers:
 %v

Parts:
`, msg.Date, msg.Subject, msg.To, msg.From, msg.ReplyTo, msg.Cc, msg.Bcc, msg.Misc)

	for i, part := range msg.Parts {
		res += fmt.Sprintf("%v: Len.: %v  Type: %v  %v\n", i, len(part.Body), part.Type.T, part.Misc)
	}

	return res
}

func checkEqual(in string, res *Msg) func(t *testing.T) {
	return func(t *testing.T) {
		msg, err := ReadMsg(strings.NewReader(in))
		if err != nil {
			t.Errorf("Parse error happened: %v", err)
			return
		}

		if fmt.Sprintf("%v", res) != fmt.Sprintf("%v", msg) {
			t.Errorf(`Parser output mismatch:
- Expected:
%v

- Actual:
%v

`, prettyPrint(msg), prettyPrint(res))
		}
	}
}

var (
	simple7bitRaw = `To: test@test
From: test <test@test>
Subject: test
Date: Tue, 8 May 2018 20:48:21 +0000
Content-Type: text/plain; charset=utf-8
Content-Transfer-Encoding: 7bit
Cc: foo <foo@foo>, bar <bar@bar>
X-CustomHeader: foo

Test! Test! Test! Test! ` + "\u5730\u9F20"
	simple7bitParsed = Msg{
		Date:    time.Date(2018, time.May, 8, 20, 48, 21, 0, time.UTC),
		To:      []mail.Address{mail.Address{"", "test@test"}},
		Subject: "test",
		From:    mail.Address{"test", "test@test"},
		ReplyTo: mail.Address{},
		Cc:      []mail.Address{mail.Address{"foo", "foo@foo"}, mail.Address{"bar", "bar@bar"}},
		Bcc:     []mail.Address{},
		Misc: mail.Header{
			"X-Customheader": []string{"foo"},
		},
		Parts: []Part{
			Part{
				BodyType{"text/plain", map[string]string{"charset": "utf-8"}},
				mail.Header{},
				[]byte("Test! Test! Test! Test! \u5730\u9F20"),
			},
		},
	}

	// See TestReadMsg for parsed struct.
	simpleBase64Raw = `To: test@test
From: test <test@test>
Subject: test
Date: Tue, 8 May 2018 20:48:21 +0000
Content-Type: text/plain; charset=utf-8
Content-Transfer-Encoding: base64
Cc: foo <foo@foo>, bar <bar@bar>
X-CustomHeader: foo

VGVzdCEgVGVzdCEgVGVzdCEgVGVzdCEg5Zyw6byg`

	simpleBase64Parsed = simple7bitParsed

	simpleQuotedPrintedParsed = simple7bitParsed
)

func TestReadMsg(t *testing.T) {
	//t.Run("multipart/7bit", checkEqual(multipart7bitRaw, &multipart7bitParsed))
	//t.Run("multipart/8bit", checkEqual(multipart8bitRaw, &multipart8bitParsed))
	//t.Run("multipart/base64", checkEqual(multipartBase64Raw, &multipartBase64Parsed))
	//t.Run("multipart/quoted-printed", checkEqual(multipartQuotedPrintedRaw, &multipartQuotedPrintedParsed))
	t.Run("simple/7bit", checkEqual(simple7bitRaw, &simple7bitParsed))
	t.Run("simple/base64", checkEqual(simple7bitRaw, &simple7bitParsed))
	//t.Run("simple/quoted-printed", checkEqual(simpleQuotedPrintedRaw, &simpleQuotedPrintedParsed))
}
