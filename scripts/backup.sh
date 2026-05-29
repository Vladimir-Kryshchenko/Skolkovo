#!/usr/bin/env bash
# backup.sh — Backup and restore utility for Skolkovo Base infrastructure.
# Supports PostgreSQL, Qdrant snapshots, and document archives.
#
# Usage:
#   ./backup.sh backup_all
#   ./backup.sh restore_all /path/to/backup_dir
#   ./backup.sh backup_postgres
#   ./backup.sh backup_qdrant
#   ./backup.sh backup_docs
#   ./backup.sh list_backups
#   ./backup.sh cleanup_old_backups 30

set -euo pipefail

# ─── Configuration ───────────────────────────────────────────────────────────
BACKUP_BASE_DIR="${BACKUP_BASE_DIR:-./backups}"
PG_HOST="${PG_HOST:-localhost}"
PG_PORT="${PG_PORT:-5432}"
PG_USER="${PG_USER:-postgres}"
PG_DB="${PG_DB:-skolkovo}"
PG_PASSWORD="${PG_PASSWORD:-}"
QDRANT_URL="${QDRANT_URL:-http://localhost:6333}"
QDRANT_COLLECTION="${QDRANT_COLLECTION:-skolkovo_docs}"
DOCS_DIR="${DOCS_DIR:-./data/docs}"
DATE_FMT="%Y%m%d_%H%M%S"
LOG_FILE="${BACKUP_BASE_DIR}/backup.log"

# ─── Helpers ─────────────────────────────────────────────────────────────────
log() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] $*"
    echo "$msg"
    mkdir -p "$BACKUP_BASE_DIR"
    echo "$msg" >> "$LOG_FILE"
}

check_cmd() {
    if ! command -v "$1" &>/dev/null; then
        log "ERROR: '$1' is not installed. Please install it first."
        exit 1
    fi
}

backup_dir_name() {
    echo "backup_$(date +"$DATE_FMT")"
}

# ─── PostgreSQL Backup ──────────────────────────────────────────────────────
backup_postgres() {
    check_cmd pg_dump
    local dir="$BACKUP_BASE_DIR/$(backup_dir_name)"
    mkdir -p "$dir/postgres"
    local dump_file="$dir/postgres/skolkovo_$(date +"$DATE_FMT").sql.gz"

    log "Starting PostgreSQL backup → $dump_file"
    export PGPASSWORD="$PG_PASSWORD"
    pg_dump -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$PG_DB" \
        --format=plain --no-owner --no-privileges \
        | gzip > "$dump_file"
    unset PGPASSWORD

    local size
    size=$(du -sh "$dump_file" | cut -f1)
    log "PostgreSQL backup complete ($size): $dump_file"
    echo "$dump_file"
}

# ─── Qdrant Snapshot ────────────────────────────────────────────────────────
backup_qdrant() {
    check_cmd curl
    local dir="$BACKUP_BASE_DIR/$(backup_dir_name)"
    mkdir -p "$dir/qdrant"
    local snapshot_file="$dir/qdrant/${QDRANT_COLLECTION}_$(date +"$DATE_FMT").snapshot"

    log "Starting Qdrant snapshot for collection '$QDRANT_COLLECTION' → $snapshot_file"

    # Create snapshot via Qdrant API
    local snapshot_response
    snapshot_response=$(curl -s -X POST \
        "${QDRANT_URL}/collections/${QDRANT_COLLECTION}/snapshots" \
        -H "Content-Type: application/json")

    local snapshot_name
    snapshot_name=$(echo "$snapshot_response" | grep -oP '"name":"\K[^"]+' || true)

    if [[ -z "$snapshot_name" ]]; then
        log "ERROR: Failed to create Qdrant snapshot. Response: $snapshot_response"
        return 1
    fi

    log "Qdrant snapshot created: $snapshot_name, downloading..."

    # Download snapshot
    curl -s -o "$snapshot_file" \
        "${QDRANT_URL}/collections/${QDRANT_COLLECTION}/snapshots/${snapshot_name}"

    local size
    size=$(du -sh "$snapshot_file" | cut -f1)
    log "Qdrant snapshot downloaded ($size): $snapshot_file"
    echo "$snapshot_file"
}

# ─── Documents Backup ───────────────────────────────────────────────────────
backup_docs() {
    check_cmd tar
    local dir="$BACKUP_BASE_DIR/$(backup_dir_name)"
    mkdir -p "$dir/docs"
    local archive_file="$dir/docs/documents_$(date +"$DATE_FMT").tar.gz"

    log "Starting documents backup → $archive_file"

    if [[ -d "$DOCS_DIR" ]]; then
        tar -czf "$archive_file" -C "$(dirname "$DOCS_DIR")" "$(basename "$DOCS_DIR")"
        local size
        size=$(du -sh "$archive_file" | cut -f1)
        log "Documents backup complete ($size): $archive_file"
    else
        log "WARNING: Documents directory '$DOCS_DIR' does not exist, creating empty marker"
        touch "$archive_file"
    fi

    echo "$archive_file"
}

# ─── Full Backup ────────────────────────────────────────────────────────────
backup_all() {
    local dir="$BACKUP_BASE_DIR/$(backup_dir_name)"
    mkdir -p "$dir"

    log "=== Starting full backup → $dir ==="

    backup_postgres
    backup_qdrant
    backup_docs

    # Calculate total size
    local total_size
    total_size=$(du -sh "$dir" | cut -f1)

    # Create manifest
    cat > "$dir/MANIFEST.txt" <<EOF
Backup manifest
Date: $(date '+%Y-%m-%d %H:%M:%S')
Host: $(hostname 2>/dev/null || echo "unknown")
PostgreSQL: ${PG_DB}@${PG_HOST}:${PG_PORT}
Qdrant: ${QDRANT_URL} / collection=${QDRANT_COLLECTION}
Docs: ${DOCS_DIR}
Total size: ${total_size}
EOF

    log "=== Full backup complete ($total_size) → $dir ==="
}

# ─── Restore ────────────────────────────────────────────────────────────────
restore_all() {
    local backup_dir="$1"

    if [[ ! -d "$backup_dir" ]]; then
        log "ERROR: Backup directory '$backup_dir' does not exist"
        exit 1
    fi

    log "=== Starting restore from $backup_dir ==="

    # Restore PostgreSQL
    local pg_file
    pg_file=$(find "$backup_dir/postgres" -name "*.sql.gz" 2>/dev/null | head -1 || true)
    if [[ -n "$pg_file" ]]; then
        log "Restoring PostgreSQL from $pg_file"
        check_cmd psql
        export PGPASSWORD="$PG_PASSWORD"
        gunzip -c "$pg_file" | psql -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$PG_DB"
        unset PGPASSWORD
        log "PostgreSQL restore complete"
    else
        log "WARNING: No PostgreSQL dump found in $backup_dir/postgres"
    fi

    # Restore Qdrant
    local qdrant_file
    qdrant_file=$(find "$backup_dir/qdrant" -name "*.snapshot" 2>/dev/null | head -1 || true)
    if [[ -n "$qdrant_file" ]]; then
        log "Restoring Qdrant from $qdrant_file"
        check_cmd curl

        local snapshot_name
        snapshot_name=$(basename "$qdrant_file")

        # Upload snapshot
        curl -s -X POST \
            "${QDRANT_URL}/collections/${QDRANT_COLLECTION}/snapshots/upload" \
            -F "snapshot=@${qdrant_file}"

        log "Qdrant restore complete"
    else
        log "WARNING: No Qdrant snapshot found in $backup_dir/qdrant"
    fi

    # Restore documents
    local docs_file
    docs_file=$(find "$backup_dir/docs" -name "*.tar.gz" 2>/dev/null | head -1 || true)
    if [[ -n "$docs_file" && -s "$docs_file" ]]; then
        log "Restoring documents from $docs_file"
        check_cmd tar
        tar -xzf "$docs_file" -C /
        log "Documents restore complete"
    else
        log "WARNING: No documents archive found in $backup_dir/docs"
    fi

    log "=== Restore complete from $backup_dir ==="
}

# ─── List Backups ───────────────────────────────────────────────────────────
list_backups() {
    if [[ ! -d "$BACKUP_BASE_DIR" ]]; then
        log "No backups found at $BACKUP_BASE_DIR"
        return 0
    fi

    log "Available backups in $BACKUP_BASE_DIR:"
    echo ""
    printf "%-40s %-15s %-12s %s\n" "NAME" "SIZE" "POSTGRES" "QDRANT"
    printf "%-40s %-15s %-12s %s\n" "$(printf '%0.s-' {1..40})" "$(printf '%0.s-' {1..15})" "$(printf '%0.s-' {1..12})" "$(printf '%0.s-' {1..10})"

    for dir in "$BACKUP_BASE_DIR"/backup_*; do
        [[ -d "$dir" ]] || continue
        local name
        name=$(basename "$dir")
        local size
        size=$(du -sh "$dir" | cut -f1)
        local has_pg="no"
        local has_qdrant="no"

        [[ -n $(find "$dir/postgres" -name "*.sql.gz" 2>/dev/null | head -1) ]] && has_pg="yes"
        [[ -n $(find "$dir/qdrant" -name "*.snapshot" 2>/dev/null | head -1) ]] && has_qdrant="yes"

        printf "%-40s %-15s %-12s %s\n" "$name" "$size" "$has_pg" "$has_qdrant"
    done
    echo ""
}

# ─── Cleanup Old Backups ────────────────────────────────────────────────────
cleanup_old_backups() {
    local days="${1:-30}"

    if [[ ! -d "$BACKUP_BASE_DIR" ]]; then
        log "No backups directory found"
        return 0
    fi

    log "Cleaning up backups older than $days days..."

    local count=0
    while IFS= read -r -d '' dir; do
        log "Removing old backup: $(basename "$dir")"
        rm -rf "$dir"
        ((count++)) || true
    done < <(find "$BACKUP_BASE_DIR" -maxdepth 1 -type d -name "backup_*" -mtime +"$days" -print0)

    log "Removed $count old backup(s)"
}

# ─── Main ────────────────────────────────────────────────────────────────────
main() {
    mkdir -p "$BACKUP_BASE_DIR"

    local cmd="${1:-}"
    shift || true

    case "$cmd" in
        backup_all)
            backup_all
            ;;
        restore_all)
            if [[ $# -lt 1 ]]; then
                log "Usage: $0 restore_all <backup_dir>"
                exit 1
            fi
            restore_all "$1"
            ;;
        backup_postgres)
            backup_postgres
            ;;
        backup_qdrant)
            backup_qdrant
            ;;
        backup_docs)
            backup_docs
            ;;
        list_backups)
            list_backups
            ;;
        cleanup_old_backups)
            cleanup_old_backups "${1:-30}"
            ;;
        *)
            echo "Usage: $0 {backup_all|restore_all|backup_postgres|backup_qdrant|backup_docs|list_backups|cleanup_old_backups}"
            echo ""
            echo "Commands:"
            echo "  backup_all              Full backup (PostgreSQL + Qdrant + docs)"
            echo "  restore_all <dir>       Restore from backup directory"
            echo "  backup_postgres         PostgreSQL backup only"
            echo "  backup_qdrant           Qdrant snapshot only"
            echo "  backup_docs             Documents archive only"
            echo "  list_backups            List available backups"
            echo "  cleanup_old_backups N   Remove backups older than N days (default: 30)"
            exit 1
            ;;
    esac
}

main "$@"
