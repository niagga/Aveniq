package schedule

// Modality says where a session takes place: online or in the practice (L61). It is
// chosen on each slot and on each series, and the series passes its own to every
// occurrence it creates.
type Modality string

const (
	ModalityOnline     Modality = "online"
	ModalityPresencial Modality = "presencial"
)

// Valid reports whether the modality is one the practice can book.
func (m Modality) Valid() bool {
	switch m {
	case ModalityOnline, ModalityPresencial:
		return true
	default:
		return false
	}
}
