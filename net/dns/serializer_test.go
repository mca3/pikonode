package dns

import (
	"bytes"
	"testing"
)

func TestSerialize(t *testing.T) {
	tests := []struct {
		name string
		exp  []byte
		data dnsMessage
	}{
		{
			"query example.com",
			[]byte{
				0xc2, 0x22, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x07, 0x65, 0x78, 0x61,
				0x6d, 0x70, 0x6c, 0x65, 0x03, 0x63, 0x6f, 0x6d,
				0x00, 0x00, 0x01, 0x00, 0x01,
			},
			dnsMessage{
				ID:      0xc222,
				QR:      true,
				Opcode:  opQuery,
				RD:      true,
				Qdcount: 1,
				Questions: []dnsQuestion{
					{
						Labels: [][]byte{[]byte("example"), []byte("com")},
						Type:   typeA,
						Class:  classIN,
					},
				},
			},
		},
		{
			"resp example.com",
			[]byte{
				0xa9, 0x56, 0x81, 0x80, 0x00, 0x01, 0x00, 0x01,
				0x00, 0x00, 0x00, 0x00, 0x07, 0x65, 0x78, 0x61,
				0x6d, 0x70, 0x6c, 0x65, 0x03, 0x63, 0x6f, 0x6d,
				0x00, 0x00, 0x01, 0x00, 0x01, /* 0xc0, 0x0c, uncomment when compression works*/
				0x07, 0x65, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65,
				0x03, 0x63, 0x6f, 0x6d,
				0x00,
				0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x3b, 0x7a,
				0x00, 0x04, 0x5d, 0xb8, 0xd8, 0x22,
			},
			dnsMessage{
				ID:      0xa956,
				QR:      false,
				Opcode:  opQuery,
				RD:      true,
				RA:      true,
				Qdcount: 1,
				Ancount: 1,
				Questions: []dnsQuestion{
					{
						Labels: [][]byte{[]byte("example"), []byte("com")},
						Type:   typeA,
						Class:  classIN,
					},
				},
				Answers: []dnsRecord{
					{
						Labels: [][]byte{[]byte("example"), []byte("com")},
						Type:   typeA,
						Class:  classIN,
						TTL:    80762,
						RData:  []byte{0x5d, 0xb8, 0xd8, 0x22},
					},
				},
			},
		},
	}

	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			_, err := v.data.serialize(buf)
			if err != nil {
				t.Fatalf("err = %v", err)
			}

			if !bytes.Equal(buf.Bytes(), v.exp) {
				t.Errorf("serialize = %v, expected %v", buf.Bytes(), v.exp)
			}
		})
	}
}
