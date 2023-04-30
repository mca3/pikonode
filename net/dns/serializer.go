package dns

// Note: the code you are about to read is *very* messy.
//
// It has not been optimized in any way, yet.
// TODO: Compress outgoing queries.

import (
	"encoding/binary"
	"io"
)

// serializeLabels writes labels to the writer.
func serializeLabels(w io.Writer, labels [][]byte) (int, error) {
	// For storing label size
	var tmp [1]byte
	n := 0

	for _, v := range labels {
		tmp[0] = byte(len(v) & 0x3f)
		s, err := w.Write(tmp[:])
		n += s
		if err != nil {
			return n, err
		}

		s, err = w.Write(v)
		n += s
		if err != nil {
			return n, err
		}
	}

	tmp[0] = 0

	s, err := w.Write(tmp[:])
	n += s
	return n, err
}

// serialize serializes the question to the writer.
func (q dnsQuestion) serialize(w io.Writer) (int, error) {
	n, err := serializeLabels(w, q.Labels)
	if err != nil {
		return n, err
	}

	var tmp [4]byte
	binary.BigEndian.PutUint16(tmp[:2], uint16(q.Type))
	binary.BigEndian.PutUint16(tmp[2:], uint16(q.Class))

	s, err := w.Write(tmp[:])
	n += s
	return n, err
}

// serialize serializes the record to the writer.
func (r dnsRecord) serialize(w io.Writer) (int, error) {
	n, err := serializeLabels(w, r.Labels)
	if err != nil {
		return n, err
	}

	var tmp [10]byte
	binary.BigEndian.PutUint16(tmp[:2], uint16(r.Type))
	binary.BigEndian.PutUint16(tmp[2:4], uint16(r.Class))
	binary.BigEndian.PutUint32(tmp[4:8], uint32(r.TTL))
	binary.BigEndian.PutUint16(tmp[8:], uint16(len(r.RData)))

	s, err := w.Write(tmp[:])
	n += s
	if err != nil {
		return n, err
	}

	s, err = w.Write(r.RData)
	n += s
	return n, err
}

// serialize serializes the DNS message to the writer.
func (m dnsMessage) serialize(w io.Writer) (int, error) {
	var tmp [12]byte

	binary.BigEndian.PutUint16(tmp[:2], uint16(m.ID))
	if !m.QR {
		tmp[2] |= 1 << 7
	}
	tmp[2] |= (uint8(m.Opcode) & 0b1111) << 3
	if m.AA {
		tmp[2] |= 0b100
	}
	if m.TC {
		tmp[2] |= 0b10
	}
	if m.RD {
		tmp[2] |= 1
	}
	if m.RA {
		tmp[3] |= 1 << 7
	}
	tmp[3] |= uint8(m.Resp) & 0b1111

	// Ignoring other values intentionally
	binary.BigEndian.PutUint16(tmp[4:6], uint16(len(m.Questions)))
	binary.BigEndian.PutUint16(tmp[6:8], uint16(len(m.Answers)))
	binary.BigEndian.PutUint16(tmp[8:10], uint16(len(m.Authority)))
	binary.BigEndian.PutUint16(tmp[10:12], uint16(len(m.Additional)))

	n, err := w.Write(tmp[:])
	if err != nil {
		return n, err
	}

	// Write out all sections

	for _, v := range m.Questions {
		s, err := v.serialize(w)
		n += s
		if err != nil {
			return n, err
		}
	}

	for _, v := range m.Answers {
		s, err := v.serialize(w)
		n += s
		if err != nil {
			return n, err
		}
	}

	for _, v := range m.Authority {
		s, err := v.serialize(w)
		n += s
		if err != nil {
			return n, err
		}
	}

	for _, v := range m.Additional {
		s, err := v.serialize(w)
		n += s
		if err != nil {
			return n, err
		}
	}

	return n, nil
}
