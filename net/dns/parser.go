package dns

// Note: the code you are about to read is *very* messy.
//
// It has not been optimized in any way, yet.

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type dnsRespCode uint8

const (
	respOk             dnsRespCode = 0
	respFormatErr      dnsRespCode = 1 // Unable to interpret query
	respServerFail     dnsRespCode = 2 // Generic server error
	respNXDomain       dnsRespCode = 3 // Domain does not exist
	respNotImplemented dnsRespCode = 4 // Feature not implemented

	// There's more, but no point writing them in.
)

type dnsOp uint8

const (
	opQuery  dnsOp = 0
	opIquery dnsOp = 1
)

type dnsType uint16

const (
	typeA    dnsType = 1  // IPv4
	typeAAAA dnsType = 28 // IPv6
)

type dnsClass uint16

const (
	classIN  dnsClass = 1
	classAny dnsClass = 255

	// No point writing the other ones, we can't use them anyway.
)

// dnsQuestion holds data for a single DNS query.
//
// You see these in the "Question" section of a DNS query.
type dnsQuestion struct {
	Labels [][]byte
	Type   dnsType
	Class  dnsClass
}

// dnsRecord holds all data for a DNS record.
//
// These are used for answers to queries (see dnsQuestion), name server
// authority information, and additional records.
type dnsRecord struct {
	Labels [][]byte
	Type   dnsType
	Class  dnsClass
	TTL    uint32
	RData  []byte
}

// dnsMessage holds a single DNS message and all related data.
type dnsMessage struct {
	ID     uint16 // ID of the message.
	QR     bool   // True when a query.
	Opcode dnsOp
	AA     bool        // Authorative Answer. Valid in responses.
	TC     bool        // TrunCation. Specifies that the response was truncated.
	RD     bool        // Recursion Desired. Valid in a query, optional server feature.
	RA     bool        // Recursion Available. Valid on response, optional server feature.
	Resp   dnsRespCode // Response code. Valid in responses, obviously.

	Qdcount uint16 // Number of queries.
	Ancount uint16 // Number of answers.
	Nscount uint16 // Number of name server resource records.
	Arcount uint16 // Number of additional records.

	Questions  []dnsQuestion
	Answers    []dnsRecord
	Authority  []dnsRecord
	Additional []dnsRecord
}

// parseLabels parses labels in a message.
func parseLabels(msg, buf []byte, depth int) (labels [][]byte, size int, err error) {
	for len(buf) > 0 {
		if buf[0] == 0 {
			size++
			break
		}

		length := int(buf[0] & 0b00111111)
		isOffset := buf[0]&0b11000000 == 192

		if isOffset && depth < 5 {
			if len(buf) < 2 {
				return labels, size, errors.New("short pointer")
			}

			length = int(binary.BigEndian.Uint16(buf[:2]) & 0x3fff)
			if length > len(msg) {
				return labels, size, errors.New("pointer out of bounds")
			}

			lbls, _, err := parseLabels(msg, msg[length:], depth+1)
			if err != nil {
				return labels, size, fmt.Errorf("resolve 0x%04x: %w", length, err)
			}
			buf = buf[2:]
			labels = append(labels, lbls...)
			size += 2

			return labels, size, nil
		} else if isOffset && depth > 5 {
			panic("too deep")
		}

		// Ensure we have enough space
		if length+1 > len(buf) {
			return labels, size, errors.New("length too long")
		}

		labels = append(labels, buf[1:length+1])
		buf = buf[length+1:]
		size += length + 1
	}

	return labels, size, nil
}

// parseRecordSection attempts to parse a record section.
func parseRecordSection(msg, section []byte, count int) (records []dnsRecord, size int, err error) {
	read := 0

	for ; count > 0 && len(section) > 0; count-- {
		labels, offs, err := parseLabels(msg, section, 0)
		if err != nil {
			return records, read, fmt.Errorf("failed to parse labels: %w", err)
		}
		read += offs
		section = section[offs:]

		if len(section) < 10 {
			// Incomplete section section
			return records, read, errors.New("incomplete section")
		}

		typ := dnsType(binary.BigEndian.Uint16(section[0:2]))
		class := dnsClass(binary.BigEndian.Uint16(section[2:4]))
		ttl := binary.BigEndian.Uint32(section[4:8])
		rdlen := binary.BigEndian.Uint16(section[8:10])

		if int(rdlen) > len(section)-10 {
			// Impossible to fill rddata
			return records, read, fmt.Errorf("rdlength too long, %d > %d", rdlen, len(section)-9)
		}

		rdata := section[10 : 10+rdlen]

		records = append(records, dnsRecord{
			Labels: labels,
			Type:   typ,
			Class:  class,
			TTL:    ttl,
			RData:  rdata,
		})

		read += 10 + int(rdlen)
		section = section[10+rdlen:]
	}

	return records, read, nil
}

// parseDNSMessage attempts to parse buf as a DNS message.
// Upon error, ok is set to false.
func parseDNSMessage(buf []byte) (msg dnsMessage, err error) {
	if len(buf) <= 12 {
		// Buffer should at least be 12 bytes long.
		return msg, errors.New("message too short")
	}

	msg.ID = binary.BigEndian.Uint16(buf[:2])
	msg.QR = buf[2]&0b10000000 == 0
	msg.Opcode = dnsOp((buf[2] >> 3) & 0b1111)
	msg.AA = buf[2]&0b00000100 != 0
	msg.TC = buf[2]&0b00000010 != 0
	msg.RD = buf[2]&0b00000001 != 0
	msg.RA = buf[3]&0b10000000 != 0
	msg.Resp = dnsRespCode(buf[3] & 0b00001111)

	msg.Qdcount = binary.BigEndian.Uint16(buf[4:6])
	msg.Ancount = binary.BigEndian.Uint16(buf[6:8])
	msg.Nscount = binary.BigEndian.Uint16(buf[8:10])
	msg.Arcount = binary.BigEndian.Uint16(buf[10:12])

	// Now we must parse questions.
	question := buf[12:]
	for i := 0; i < int(msg.Qdcount) && len(question) > 0; i++ {
		// Parse labels
		labels, offs, err := parseLabels(buf, question, 0)
		if err != nil {
			return msg, fmt.Errorf("failed to parse question labels: %w", err)
		}
		question = question[offs:]

		// Fetch type and class
		if len(question) < 4 {
			// Incomplete question section
			return msg, errors.New("incomplete question section")
		}

		typ := dnsType(binary.BigEndian.Uint16(question[0:2]))
		class := dnsClass(binary.BigEndian.Uint16(question[2:4]))

		msg.Questions = append(msg.Questions, dnsQuestion{
			Labels: labels,
			Type:   typ,
			Class:  class,
		})
		question = question[4:]
	}

	// Answers...
	offs := 0
	msg.Answers, offs, err = parseRecordSection(buf, question, int(msg.Ancount))
	if err != nil {
		return msg, fmt.Errorf("failed parsing answers: %w", err)
	}
	question = question[offs:]

	// Authority...
	msg.Authority, offs, err = parseRecordSection(buf, question, int(msg.Nscount))
	if err != nil {
		return msg, fmt.Errorf("failed parsing authority: %w", err)
	}
	question = question[offs:]

	// Additional...
	msg.Additional, _, err = parseRecordSection(buf, question, int(msg.Arcount))
	if err != nil {
		return msg, fmt.Errorf("failed parsing additional records: %w", err)
	}

	return msg, nil
}
