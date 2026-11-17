package console_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateAnnouncementsPopup(t *testing.T) {
	valid := `[{"content":"notice","publishDate":"2026-11-17T02:00:00Z","popup":true}]`
	require.NoError(t, ValidateConsoleSettings(valid, "Announcements"))

	omitted := `[{"content":"notice","publishDate":"2026-11-17T02:00:00Z"}]`
	require.NoError(t, ValidateConsoleSettings(omitted, "Announcements"))

	invalid := `[{"content":"notice","publishDate":"2026-11-17T02:00:00Z","popup":"yes"}]`
	require.Error(t, ValidateConsoleSettings(invalid, "Announcements"))
}
