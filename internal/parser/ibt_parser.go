package parser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juanmferreira93/iracing-agent/internal/domain"
)

type IBTParser struct{}

type ibtHeader struct {
	Version           int32
	Status            int32
	TickRate          int32
	SessionInfoUpdate int32
	SessionInfoLen    int32
	SessionInfoOffset int32
	NumVars           int32
	VarHeaderOffset   int32
	NumBuf            int32
	BufLen            int32
	Pad               [2]int32
	VarBufs           [3]ibtVarBuffer
}

type ibtVarHeader struct {
	VarType     int32
	Offset      int32
	Count       int32
	CountAsTime bool
	Pad         [3]byte
	Name        [32]byte
	Desc        [64]byte
	Unit        [32]byte
}

type ibtVarBuffer struct {
	TickCount int32
	BufOffset int32
	Pad       [2]int32
}

type lapStats struct {
	startTime   float64
	endTime     float64
	speedSum    float64
	speedCount  int
	initialized bool
}

func NewIBTParser() *IBTParser {
	return &IBTParser{}
}

func (p *IBTParser) ParseFile(_ context.Context, filePath string) (*domain.UploadBundle, error) {
	if !strings.EqualFold(filepath.Ext(filePath), ".ibt") {
		return nil, fmt.Errorf("unsupported file extension: %s", filepath.Ext(filePath))
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	if len(data) < binary.Size(ibtHeader{}) {
		return nil, fmt.Errorf("invalid ibt file (too small)")
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	header, err := parseHeader(data)
	if err != nil {
		return nil, err
	}

	varHeaders, err := parseVarHeaders(data, header)
	if err != nil {
		return nil, err
	}

	lookup := make(map[string]ibtVarHeader, len(varHeaders))
	for _, item := range varHeaders {
		lookup[cString(item.Name[:])] = item
	}

	startOffset := latestBufOffset(header)
	if startOffset <= 0 || startOffset >= len(data) {
		return nil, fmt.Errorf("invalid ibt var buffer start offset")
	}
	if header.BufLen <= 0 {
		return nil, fmt.Errorf("invalid ibt buffer length")
	}

	rows := (len(data) - startOffset) / int(header.BufLen)
	if rows <= 0 {
		return nil, fmt.Errorf("no telemetry rows found")
	}

	samples := make([]domain.Sample, 0, rows)
	lapsAcc := map[int]*lapStats{}
	maxSessionTime := 0.0

	for row := 0; row < rows; row++ {
		rowStart := startOffset + row*int(header.BufLen)
		rowEnd := rowStart + int(header.BufLen)
		if rowEnd > len(data) {
			break
		}
		buf := data[rowStart:rowEnd]

		sessionTime, hasSessionTime := getFloat64Var(buf, lookup, "SessionTime")
		if !hasSessionTime {
			sessionTime = float64(row) / float64(nonZero(header.TickRate, 60))
		}
		if sessionTime > maxSessionTime {
			maxSessionTime = sessionTime
		}

		speedMS, _ := getFloat32Var(buf, lookup, "Speed")
		rpm, _ := getFloat32Var(buf, lookup, "RPM")
		throttle, _ := getFloat32Var(buf, lookup, "Throttle")
		brake, _ := getFloat32Var(buf, lookup, "Brake")
		gear, _ := getInt32Var(buf, lookup, "Gear")
		lapNo, hasLap := getInt32Var(buf, lookup, "Lap")

		speedK := float64(speedMS) * 3.6

		if hasLap && lapNo > 0 {
			lap := int(lapNo)
			stats := lapsAcc[lap]
			if stats == nil {
				stats = &lapStats{startTime: sessionTime, endTime: sessionTime, initialized: true}
				lapsAcc[lap] = stats
			}
			if !stats.initialized {
				stats.startTime = sessionTime
				stats.endTime = sessionTime
				stats.initialized = true
			}
			if sessionTime < stats.startTime {
				stats.startTime = sessionTime
			}
			if sessionTime > stats.endTime {
				stats.endTime = sessionTime
			}
			stats.speedSum += speedK
			stats.speedCount++
		}

		samples = append(samples, domain.Sample{
			TimestampS: sessionTime,
			SpeedK:     speedK,
			RPM:        float64(rpm),
			Throttle:   float64(throttle),
			Brake:      float64(brake),
			Gear:       int(gear),
		})
	}

	hash := hashBytes(data)
	sessionYAML := extractSessionYAML(data, header)

	track := extractSessionValue(sessionYAML, "TrackDisplayName")
	if track == "" {
		track = "unknown"
	}
	car := extractCarName(sessionYAML)
	if car == "" {
		car = extractSessionValue(sessionYAML, "DriverCarSLFirstName")
	}
	if car == "" {
		car = "unknown"
	}

	laps := buildLaps(lapsAcc)

	bundle := &domain.UploadBundle{
		Session: domain.Session{
			ExternalID: hash,
			SourceFile: filePath,
			FileHash:   hash,
			FileSize:   info.Size(),
			CapturedAt: info.ModTime().UTC(),
			Track:      track,
			Car:        car,
			DurationS:  int(maxSessionTime),
		},
		Laps:    laps,
		Samples: samples,
	}

	return bundle, nil
}

func parseHeader(data []byte) (ibtHeader, error) {
	var header ibtHeader
	size := binary.Size(ibtHeader{})
	if err := binary.Read(bytes.NewReader(data[:size]), binary.LittleEndian, &header); err != nil {
		return ibtHeader{}, err
	}
	return header, nil
}

func parseVarHeaders(data []byte, header ibtHeader) ([]ibtVarHeader, error) {
	if header.NumVars <= 0 {
		return nil, fmt.Errorf("invalid var header count")
	}
	offset := int(header.VarHeaderOffset)
	entrySize := binary.Size(ibtVarHeader{})
	if offset < 0 || offset >= len(data) {
		return nil, fmt.Errorf("invalid var header offset")
	}

	out := make([]ibtVarHeader, 0, int(header.NumVars))
	for i := 0; i < int(header.NumVars); i++ {
		start := offset + i*entrySize
		end := start + entrySize
		if end > len(data) {
			return nil, fmt.Errorf("ibt var headers truncated")
		}

		var vh ibtVarHeader
		if err := binary.Read(bytes.NewReader(data[start:end]), binary.LittleEndian, &vh); err != nil {
			return nil, err
		}
		out = append(out, vh)
	}

	return out, nil
}

func latestBufOffset(header ibtHeader) int {
	if header.NumBuf <= 0 {
		return 0
	}
	latest := header.VarBufs[0]
	for i := 1; i < int(min32(header.NumBuf, 3)); i++ {
		if header.VarBufs[i].TickCount > latest.TickCount {
			latest = header.VarBufs[i]
		}
	}
	return int(latest.BufOffset)
}

func getFloat32Var(buf []byte, vars map[string]ibtVarHeader, name string) (float32, bool) {
	vh, ok := vars[name]
	if !ok {
		return 0, false
	}
	o := int(vh.Offset)
	if o < 0 || o+4 > len(buf) {
		return 0, false
	}
	bits := binary.LittleEndian.Uint32(buf[o : o+4])
	return float32FromBits(bits), true
}

func getFloat64Var(buf []byte, vars map[string]ibtVarHeader, name string) (float64, bool) {
	vh, ok := vars[name]
	if !ok {
		return 0, false
	}
	o := int(vh.Offset)
	if o < 0 || o+8 > len(buf) {
		return 0, false
	}
	bits := binary.LittleEndian.Uint64(buf[o : o+8])
	return float64FromBits(bits), true
}

func getInt32Var(buf []byte, vars map[string]ibtVarHeader, name string) (int32, bool) {
	vh, ok := vars[name]
	if !ok {
		return 0, false
	}
	o := int(vh.Offset)
	if o < 0 || o+4 > len(buf) {
		return 0, false
	}
	return int32(binary.LittleEndian.Uint32(buf[o : o+4])), true
}

func cString(b []byte) string {
	idx := 0
	for idx < len(b) && b[idx] != 0 {
		idx++
	}
	return string(b[:idx])
}

func buildLaps(acc map[int]*lapStats) []domain.Lap {
	if len(acc) == 0 {
		return []domain.Lap{}
	}
	keys := make([]int, 0, len(acc))
	for lap := range acc {
		keys = append(keys, lap)
	}
	sort.Ints(keys)

	laps := make([]domain.Lap, 0, len(keys))
	for _, lapNumber := range keys {
		stats := acc[lapNumber]
		if stats == nil {
			continue
		}
		avgSpeed := 0.0
		if stats.speedCount > 0 {
			avgSpeed = stats.speedSum / float64(stats.speedCount)
		}
		lapTime := stats.endTime - stats.startTime
		if lapTime < 0 {
			lapTime = 0
		}
		laps = append(laps, domain.Lap{
			Number:        lapNumber,
			LapTimeS:      lapTime,
			AverageSpeedK: avgSpeed,
		})
	}
	return laps
}

func extractSessionYAML(data []byte, header ibtHeader) string {
	offset := int(header.SessionInfoOffset)
	length := int(header.SessionInfoLen)
	if offset <= 0 || length <= 0 || offset+length > len(data) {
		return ""
	}
	return string(data[offset : offset+length])
}

func extractSessionValue(yamlText string, key string) string {
	if yamlText == "" {
		return ""
	}
	needle := key + ":"
	for _, rawLine := range strings.Split(yamlText, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, needle) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, needle))
		value = strings.Trim(value, `"`)
		return value
	}
	return ""
}

func extractCarName(yamlText string) string {
	if yamlText == "" {
		return ""
	}

	driverIdx := extractSessionValue(yamlText, "DriverCarIdx")
	preferredIdx := strings.TrimSpace(driverIdx)

	lines := strings.Split(yamlText, "\n")
	currentIdx := ""
	firstSeen := ""

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "CarIdx:") {
			currentIdx = strings.TrimSpace(strings.TrimPrefix(line, "CarIdx:"))
			continue
		}

		if strings.HasPrefix(line, "CarScreenName:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "CarScreenName:"))
			name = strings.Trim(name, `"`)
			if name == "" {
				continue
			}
			if firstSeen == "" {
				firstSeen = name
			}
			if preferredIdx != "" && currentIdx == preferredIdx {
				return name
			}
		}
	}

	if firstSeen != "" {
		return firstSeen
	}

	return extractSessionValue(yamlText, "CarScreenName")
}

func nonZero(value int32, fallback int32) int32 {
	if value == 0 {
		return fallback
	}
	return value
}

func min32(a int32, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func float32FromBits(bits uint32) float32 {
	return math.Float32frombits(bits)
}

func float64FromBits(bits uint64) float64 {
	return math.Float64frombits(bits)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
