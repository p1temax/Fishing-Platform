package utils

import (
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"sync"

	"golang.org/x/text/encoding/simplifiedchinese"
)

const (
	qqwryIndexLength = 7
	qqwryRedirect1   = 0x01
	qqwryRedirect2   = 0x02
)

var (
	ipLocationMu sync.RWMutex
	ipLocationDB *QQWry
)

type QQWry struct {
	data        []byte
	firstIndex  uint32
	lastIndex   uint32
	recordCount uint32
}

type IPLocation struct {
	Country string
	Area    string
}

func InitIPLocationDB(data []byte) error {
	db, err := NewQQWry(data)
	if err != nil {
		return err
	}

	ipLocationMu.Lock()
	ipLocationDB = db
	ipLocationMu.Unlock()
	return nil
}

func LookupIPLocation(ipText string) string {
	ipLocationMu.RLock()
	db := ipLocationDB
	ipLocationMu.RUnlock()
	if db == nil {
		return ""
	}

	location, ok := db.Find(ipText)
	if !ok {
		return ""
	}
	return location.String()
}

func NewQQWry(data []byte) (*QQWry, error) {
	if len(data) < 8 {
		return nil, errors.New("qqwry data is too small")
	}

	firstIndex := binary.LittleEndian.Uint32(data[0:4])
	lastIndex := binary.LittleEndian.Uint32(data[4:8])
	if firstIndex < 8 || lastIndex < firstIndex || int(lastIndex)+qqwryIndexLength > len(data) {
		return nil, errors.New("invalid qqwry index range")
	}

	recordCount := (lastIndex-firstIndex)/qqwryIndexLength + 1
	if recordCount == 0 {
		return nil, errors.New("qqwry has no records")
	}

	return &QQWry{
		data:        data,
		firstIndex:  firstIndex,
		lastIndex:   lastIndex,
		recordCount: recordCount,
	}, nil
}

func (q *QQWry) Find(ipText string) (IPLocation, bool) {
	ip := net.ParseIP(strings.TrimSpace(ipText))
	if ip == nil {
		return IPLocation{}, false
	}

	ipv4 := ip.To4()
	if ipv4 == nil {
		return IPLocation{}, false
	}
	ipValue := binary.LittleEndian.Uint32([]byte{ipv4[3], ipv4[2], ipv4[1], ipv4[0]})

	var left uint32
	right := q.recordCount
	for left < right {
		mid := left + (right-left)/2
		indexOffset := q.firstIndex + mid*qqwryIndexLength
		startIP := q.readUint32(indexOffset)

		if ipValue < startIP {
			right = mid
			continue
		}

		recordOffset := q.readUint24(indexOffset + 4)
		endIP := q.readUint32(recordOffset)
		if ipValue <= endIP {
			return q.readLocation(recordOffset + 4), true
		}

		left = mid + 1
	}

	return IPLocation{}, false
}

func (l IPLocation) String() string {
	parts := make([]string, 0, 2)
	for _, value := range []string{l.Country, l.Area} {
		value = cleanQQWryText(value)
		if value == "" {
			continue
		}
		if len(parts) == 0 || parts[len(parts)-1] != value {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " ")
}

func (q *QQWry) readLocation(offset uint32) IPLocation {
	mode := q.readByte(offset)
	switch mode {
	case qqwryRedirect1:
		countryOffset := q.readUint24(offset + 1)
		countryMode := q.readByte(countryOffset)
		if countryMode == qqwryRedirect2 {
			return IPLocation{
				Country: q.readString(q.readUint24(countryOffset + 1)),
				Area:    q.readArea(countryOffset + 4),
			}
		}

		country, nextOffset := q.readStringWithEnd(countryOffset)
		return IPLocation{
			Country: country,
			Area:    q.readArea(nextOffset),
		}
	case qqwryRedirect2:
		return IPLocation{
			Country: q.readString(q.readUint24(offset + 1)),
			Area:    q.readArea(offset + 4),
		}
	default:
		country, nextOffset := q.readStringWithEnd(offset)
		return IPLocation{
			Country: country,
			Area:    q.readArea(nextOffset),
		}
	}
}

func (q *QQWry) readArea(offset uint32) string {
	mode := q.readByte(offset)
	if mode == qqwryRedirect1 || mode == qqwryRedirect2 {
		return q.readString(q.readUint24(offset + 1))
	}
	return q.readString(offset)
}

func (q *QQWry) readString(offset uint32) string {
	value, _ := q.readStringWithEnd(offset)
	return value
}

func (q *QQWry) readStringWithEnd(offset uint32) (string, uint32) {
	if int(offset) >= len(q.data) {
		return "", offset
	}

	end := offset
	for int(end) < len(q.data) && q.data[end] != 0 {
		end++
	}
	if end == offset {
		return "", end + 1
	}

	value, err := simplifiedchinese.GBK.NewDecoder().String(string(q.data[offset:end]))
	if err != nil {
		value = string(q.data[offset:end])
	}
	return strings.TrimSpace(value), end + 1
}

func (q *QQWry) readByte(offset uint32) byte {
	if int(offset) >= len(q.data) {
		return 0
	}
	return q.data[offset]
}

func (q *QQWry) readUint24(offset uint32) uint32 {
	if int(offset)+3 > len(q.data) {
		return 0
	}
	return uint32(q.data[offset]) | uint32(q.data[offset+1])<<8 | uint32(q.data[offset+2])<<16
}

func (q *QQWry) readUint32(offset uint32) uint32 {
	if int(offset)+4 > len(q.data) {
		return 0
	}
	return binary.LittleEndian.Uint32(q.data[offset : offset+4])
}

func cleanQQWryText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "CZ88.NET") {
		return ""
	}
	value = strings.ReplaceAll(value, "CZ88.NET", "")
	value = strings.ReplaceAll(value, "纯真网络", "")
	return strings.TrimSpace(value)
}
