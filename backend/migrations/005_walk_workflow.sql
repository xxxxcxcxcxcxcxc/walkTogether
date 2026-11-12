ALTER TABLE meeting_history DROP CONSTRAINT IF EXISTS meeting_history_event_type_check;
ALTER TABLE meeting_history ADD CONSTRAINT meeting_history_event_type_check
    CHECK (event_type IN (
        'created', 'time_proposed', 'time_selected', 'participant_accepted',
        'participant_rejected', 'confirmed', 'rescheduled', 'cancelled', 'expired', 'completed'
    ));

CREATE INDEX IF NOT EXISTS idx_meeting_participants_user_status
    ON meeting_participants (user_id, status, meeting_id);
