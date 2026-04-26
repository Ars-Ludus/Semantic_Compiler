module semcom_orchestrator

go 1.26.2

replace (
	github.com/ars/semantic_store => ../semcom_store
	github.com/ars/semcom_retrieve => ../semcom_retrieve
	semcom_embed => ../semcom_embed
	semcom_internal => ../internal
)

require (
	github.com/ars/semantic_store v0.0.0-00010101000000-000000000000
	github.com/ars/semcom_retrieve v0.0.0-00010101000000-000000000000
	semcom_embed v0.0.0-00010101000000-000000000000
	semcom_internal v0.0.0-00010101000000-000000000000
)

require (
	github.com/RoaringBitmap/roaring v1.9.4 // indirect
	github.com/bits-and-blooms/bitset v1.12.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.9.2 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mschoch/smat v0.2.0 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	modernc.org/libc v1.72.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.49.1 // indirect
)
