package templates

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}

	b := id.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func bookLayoutClass(hasCover bool) string {
	if !hasCover {
		return "max-w-3xl"
	}
	return "grid gap-8 md:grid-cols-[minmax(0,18rem)_minmax(0,1fr)]"
}
