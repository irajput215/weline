package booking

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteExpired(t *testing.T) {
	f := newFx(t)
	old, err := f.svc.Create(ctx, f.org, f.staff, NewAppointment{ContactID: f.patient, ServiceID: f.consult, LocationID: f.bondi,
		PractitionerID: f.ada, StartsAt: syd(2027, 3, 8, 9, 0)})
	require.NoError(t, err)
	recent, err := f.svc.Create(ctx, f.org, f.staff, NewAppointment{ContactID: f.patient, ServiceID: f.consult, LocationID: f.bondi,
		PractitionerID: f.ada, StartsAt: syd(2027, 3, 8, 9, 30)})
	require.NoError(t, err)
	_, err = f.svc.SetStatus(ctx, f.org, f.staff, old.ID, StatusArrived, "")
	require.NoError(t, err)
	// Move the first appointment a month earlier, then stand 7 years after the
	// second one ended, less a day: only the first is past retention.
	require.NoError(t, f.db.Exec(`UPDATE wl.booking_appointments SET starts_at = starts_at - interval '30 days',
		ends_at = ends_at - interval '30 days' WHERE id = ?`, old.ID).Error)
	f.svc.now = func() time.Time { return recent.EndsAt.AddDate(7, 0, -1) }

	n, err := f.svc.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	var appts, history, kept int64
	require.NoError(t, f.db.Table("wl.booking_appointments").Where("id = ?", old.ID).Count(&appts).Error)
	require.NoError(t, f.db.Table("wl.booking_status_history").Where("appointment_id = ?", old.ID).Count(&history).Error)
	require.NoError(t, f.db.Table("wl.booking_appointments").Where("id = ?", recent.ID).Count(&kept).Error)
	assert.Zero(t, appts, "past retention: deleted")
	assert.Zero(t, history, "with its history")
	assert.Equal(t, int64(1), kept, "within 7 years: kept")
}
