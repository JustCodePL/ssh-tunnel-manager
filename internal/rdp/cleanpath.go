package rdp

import (
	"encoding/asn1"
	"errors"
	"fmt"
	"io"
)

// RDCleanPath is the IronRDP extension that lets a browser client delegate the
// X.224 exchange and the TLS upgrade to a proxy. Every PDU is a DER SEQUENCE of
// EXPLICIT context-tagged fields.
const (
	cleanPathVersion = 3390

	fieldVersion    = 0
	fieldError      = 1
	fieldX224       = 6
	fieldCertChain  = 7
	fieldServerAddr = 9

	asn1ClassContext = 2
	asn1TagSequence  = 16

	// Error codes defined by ironrdp-rdcleanpath. Codes from 0x4000 to 0x7FFF
	// are reserved for the proxy implementation.
	errCodeGeneral           = 1
	errCodeNegotiation       = 2
	errCodeTargetUnreachable = 103
	errCodeTLSHandshake      = 105
	errCodeCertChanged       = 0x4001

	maxTPKTLength = 4096
)

// parseCleanPathRequest validates a client request and returns its X.224
// connection request. The destination field is deliberately ignored: the bridge
// connects only to the target registered with the session token.
func parseCleanPathRequest(pdu []byte) ([]byte, error) {
	var outer asn1.RawValue
	if rest, err := asn1.Unmarshal(pdu, &outer); err != nil {
		return nil, fmt.Errorf("parsing RDCleanPath PDU: %w", err)
	} else if len(rest) != 0 {
		return nil, errors.New("trailing data after RDCleanPath PDU")
	}
	if outer.Class != asn1.ClassUniversal || outer.Tag != asn1TagSequence || !outer.IsCompound {
		return nil, errors.New("RDCleanPath PDU is not a sequence")
	}

	var version int
	var x224 []byte
	rest := outer.Bytes
	for len(rest) > 0 {
		var field asn1.RawValue
		next, err := asn1.Unmarshal(rest, &field)
		if err != nil {
			return nil, fmt.Errorf("parsing RDCleanPath field: %w", err)
		}
		rest = next
		if field.Class != asn1ClassContext {
			continue
		}
		switch field.Tag {
		case fieldVersion:
			if _, err := asn1.Unmarshal(field.Bytes, &version); err != nil {
				return nil, fmt.Errorf("parsing RDCleanPath version: %w", err)
			}
		case fieldX224:
			if _, err := asn1.Unmarshal(field.Bytes, &x224); err != nil {
				return nil, fmt.Errorf("parsing RDCleanPath X.224 field: %w", err)
			}
		}
	}
	if version != cleanPathVersion {
		return nil, fmt.Errorf("unsupported RDCleanPath version %d", version)
	}
	if len(x224) == 0 {
		return nil, errors.New("RDCleanPath request has no X.224 connection request")
	}
	return x224, nil
}

func marshalRaw(class, tag int, compound bool, body []byte) ([]byte, error) {
	return asn1.Marshal(asn1.RawValue{Class: class, Tag: tag, IsCompound: compound, Bytes: body})
}

// explicitField wraps an already encoded value in an EXPLICIT context tag.
func explicitField(tag int, encoded []byte) ([]byte, error) {
	return marshalRaw(asn1ClassContext, tag, true, encoded)
}

func joinPDU(fields ...[]byte) ([]byte, error) {
	var body []byte
	for _, field := range fields {
		body = append(body, field...)
	}
	return marshalRaw(asn1.ClassUniversal, asn1TagSequence, true, body)
}

func encodedVersion() ([]byte, error) {
	value, err := asn1.Marshal(cleanPathVersion)
	if err != nil {
		return nil, err
	}
	return explicitField(fieldVersion, value)
}

func encodedX224(x224 []byte) ([]byte, error) {
	value, err := asn1.Marshal(x224)
	if err != nil {
		return nil, err
	}
	return explicitField(fieldX224, value)
}

// encodeCleanPathResponse builds the proxy response handed to the client after
// the server accepted X.224 and completed the TLS handshake.
func encodeCleanPathResponse(x224 []byte, chain [][]byte, serverAddr string) ([]byte, error) {
	version, err := encodedVersion()
	if err != nil {
		return nil, err
	}
	response, err := encodedX224(x224)
	if err != nil {
		return nil, err
	}
	var certs []byte
	for _, cert := range chain {
		encoded, err := asn1.Marshal(cert)
		if err != nil {
			return nil, err
		}
		certs = append(certs, encoded...)
	}
	certSeq, err := marshalRaw(asn1.ClassUniversal, asn1TagSequence, true, certs)
	if err != nil {
		return nil, err
	}
	chainField, err := explicitField(fieldCertChain, certSeq)
	if err != nil {
		return nil, err
	}
	addr, err := asn1.MarshalWithParams(serverAddr, "utf8")
	if err != nil {
		return nil, err
	}
	addrField, err := explicitField(fieldServerAddr, addr)
	if err != nil {
		return nil, err
	}
	return joinPDU(version, response, chainField, addrField)
}

// encodeCleanPathError builds an error PDU. x224 is set only for negotiation
// errors, where it carries the server's negotiation failure.
func encodeCleanPathError(code int, x224 []byte) ([]byte, error) {
	version, err := encodedVersion()
	if err != nil {
		return nil, err
	}
	value, err := asn1.Marshal(code)
	if err != nil {
		return nil, err
	}
	codeField, err := explicitField(0, value)
	if err != nil {
		return nil, err
	}
	errSeq, err := marshalRaw(asn1.ClassUniversal, asn1TagSequence, true, codeField)
	if err != nil {
		return nil, err
	}
	errField, err := explicitField(fieldError, errSeq)
	if err != nil {
		return nil, err
	}
	if len(x224) == 0 {
		return joinPDU(version, errField)
	}
	response, err := encodedX224(x224)
	if err != nil {
		return nil, err
	}
	return joinPDU(version, errField, response)
}

// readTPKT reads one complete TPKT packet.
func readTPKT(r io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("reading TPKT header: %w", err)
	}
	if header[0] != 3 {
		return nil, fmt.Errorf("not a TPKT packet (version %d)", header[0])
	}
	length := int(header[2])<<8 | int(header[3])
	if length < len(header) || length > maxTPKTLength {
		return nil, fmt.Errorf("invalid TPKT length %d", length)
	}
	packet := make([]byte, length)
	copy(packet, header)
	if _, err := io.ReadFull(r, packet[len(header):]); err != nil {
		return nil, fmt.Errorf("reading TPKT body: %w", err)
	}
	return packet, nil
}

// isConnectionConfirm reports whether a TPKT packet carries an X.224
// connection confirm. A negotiation failure is also a connection confirm.
func isConnectionConfirm(packet []byte) bool {
	const x224CodeOffset = 5
	return len(packet) > x224CodeOffset && packet[x224CodeOffset]&0xF0 == 0xD0
}

// isNegotiationFailure reports whether a connection confirm carries an
// RDP_NEG_FAILURE structure.
func isNegotiationFailure(packet []byte) bool {
	const negTypeOffset = 11
	return len(packet) > negTypeOffset && packet[negTypeOffset] == 3
}
