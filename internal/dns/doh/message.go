package doh

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"

	"golang.org/x/net/dns/dnsmessage"
)

const (
	mediaType = "application/dns-message"
	// maxMessageSize is the largest DNS message. Larger responses are rejected.
	maxMessageSize = 65535
	// paddingBlock is the query padding block size recommended by RFC 8467.
	paddingBlock = 128
	// optionPadding is the EDNS(0) Padding option code (RFC 7830).
	optionPadding = 12
	// udpPayloadSize goes in the OPT record. It means nothing over DoH, but the record needs one.
	udpPayloadSize = 1232
)

// packQuery builds the wire form of a query for q: ID 0, as RFC 8484 recommends so that
// HTTP caches can share answers, and RD set. The OPT record carries the Padding option,
// which pads the message to a multiple of paddingBlock bytes, and no ECS option, so the
// server applies its own ECS policy.
func packQuery(q dnsmessage.Question) ([]byte, error) {
	var optHeader dnsmessage.ResourceHeader
	if err := optHeader.SetEDNS0(udpPayloadSize, dnsmessage.RCodeSuccess, false); err != nil {
		return nil, err
	}
	opt := &dnsmessage.OPTResource{Options: []dnsmessage.Option{{Code: optionPadding}}}
	msg := dnsmessage.Message{
		Header:      dnsmessage.Header{RecursionDesired: true},
		Questions:   []dnsmessage.Question{q},
		Additionals: []dnsmessage.Resource{{Header: optHeader, Body: opt}},
	}

	unpadded, err := msg.Pack()
	if err != nil {
		return nil, err
	}
	// The empty option is already in unpadded, so the message grows by exactly len(Data).
	opt.Options[0].Data = make([]byte, (paddingBlock-len(unpadded)%paddingBlock)%paddingBlock)
	return msg.Pack()
}

// parseResponse checks resp and its body against the question that was asked, and
// returns the parsed message.
func parseResponse(resp *http.Response, body []byte, q dnsmessage.Question) (*dnsmessage.Message, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %q", resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	if mt, _, err := mime.ParseMediaType(contentType); err != nil || mt != mediaType {
		return nil, fmt.Errorf("unexpected content type %q", contentType)
	}
	if len(body) > maxMessageSize {
		return nil, errors.New("response larger than a DNS message")
	}

	var msg dnsmessage.Message
	if err := msg.Unpack(body); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if !msg.Response || msg.ID != 0 {
		return nil, errors.New("response isn't an answer to the query")
	}
	if len(msg.Questions) != 1 || !sameQuestion(msg.Questions[0], q) {
		return nil, errors.New("response answers a different question")
	}
	return &msg, nil
}

func sameQuestion(a, b dnsmessage.Question) bool {
	return a.Type == b.Type && a.Class == b.Class && strings.EqualFold(a.Name.String(), b.Name.String())
}
