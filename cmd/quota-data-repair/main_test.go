package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRepairTimeSupportsShanghaiDateAndUnix(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	start, err := parseRepairTime("2026-08-25", location, false)
	require.NoError(t, err)
	end, err := parseRepairTime("2026-08-25", location, true)
	require.NoError(t, err)
	unix, err := parseRepairTime("1787587200", location, false)
	require.NoError(t, err)

	assert.Equal(t, int64(1787587200), start)
	assert.Equal(t, int64(1787673599), end)
	assert.Equal(t, start, unix)
}

func TestParseRepairTimeRejectsEmptyAndNonPositiveValues(t *testing.T) {
	location := time.FixedZone("test", 8*60*60)
	for _, value := range []string{"", "0", "-1", "not-a-time"} {
		_, err := parseRepairTime(value, location, false)
		assert.Error(t, err, "value=%q", value)
	}
}
