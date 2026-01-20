# Beads - Clean Room Technical Specification

**Version:** 1.0
**Purpose:** This specification describes the requirements and behaviors for implementing a distributed, git-backed issue tracking system designed for AI agents.

---

## 1. Introduction

### 1.1 Overview

Beads is a distributed issue tracking system that stores data in git repositories using a human-readable format. It is designed to serve as persistent, structured memory for AI agents, replacing unstructured markdown with dependency-aware graphs.

### 1.2 Design Goals

1. **Git-Native Persistence**: All data must be stored in formats that work well with git (text-based, diff-friendly, merge-capable)
2. **Distributed Operation**: Multiple clones of a repository must be able to work independently and merge changes
3. **Offline-First**: Full functionality without network connectivity
4. **Agent-Optimized**: Designed for programmatic access by AI agents with limited context windows
5. **Atomic Operations**: All operations must maintain data integrity

### 1.3 Terminology

| Term | Definition |
|------|------------|
| **Issue** | The primary work item entity |
| **Dependency** | A directed relationship between two issues |
| **Tombstone** | A soft-deleted issue that remains in storage for distributed consistency |
| **Molecule** | A reusable template for common work patterns |
| **Beads Directory** | The `.beads/` folder containing all tracker data |

---

## 2. Data Model

### 2.1 Issue Entity

An Issue is the primary entity representing a unit of work. Implementations MUST support all required fields and SHOULD support optional fields.

#### 2.1.1 Required Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `id` | string | Unique, immutable | Unique identifier for the issue |
| `title` | string | Non-empty, max 500 chars | Brief summary of the issue |
| `status` | enum | See §2.1.4 | Current workflow state |
| `created_at` | timestamp | RFC3339 format | When the issue was created |
| `updated_at` | timestamp | RFC3339 format | When the issue was last modified |

#### 2.1.2 Optional Content Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `description` | string | Max 64KB | Detailed description |
| `design` | string | Max 64KB | Technical design notes |
| `acceptance_criteria` | string | Max 16KB | Definition of done |
| `notes` | string | Max 64KB | Freeform notes (append-only merge behavior) |

#### 2.1.3 Optional Metadata Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `priority` | integer | 0-4 inclusive | 0=critical, 4=low; default=2 |
| `issue_type` | string | Suggested: task, bug, feature, epic, story | Classification of work |
| `assignee` | string | Max 256 chars | Current worker identifier |
| `owner` | string | Max 256 chars | Accountable party identifier |
| `estimated_minutes` | integer | Non-negative | Time estimate in minutes |
| `due_at` | timestamp | RFC3339 format | Deadline |
| `defer_until` | timestamp | RFC3339 format | Do not surface until this time |
| `external_ref` | string | Valid URI | Link to external system |
| `source_system` | string | Max 64 chars | Origin system identifier |
| `created_by` | string | Max 256 chars | Creator identifier |

#### 2.1.4 Status Values

Implementations MUST support these status values:

| Status | Description | Transitions To |
|--------|-------------|----------------|
| `open` | Available for work | in-progress, blocked, deferred, closed, tombstone |
| `in-progress` | Currently being worked | open, blocked, closed, tombstone |
| `blocked` | Waiting on dependencies | open, in-progress, closed, tombstone |
| `deferred` | Postponed | open, tombstone |
| `closed` | Completed or resolved | open (reopen), tombstone |
| `tombstone` | Soft-deleted | (terminal state) |

#### 2.1.5 Tombstone Fields

When `status` = `tombstone`, these fields MUST be populated:

| Field | Type | Description |
|-------|------|-------------|
| `deleted_at` | timestamp | When deletion occurred |
| `deleted_by` | string | Who performed deletion |
| `delete_reason` | string | Why the issue was deleted |
| `original_type` | string | Issue type before deletion |

#### 2.1.6 Closure Fields

When `status` = `closed`, these fields SHOULD be populated:

| Field | Type | Description |
|-------|------|-------------|
| `closed_at` | timestamp | When closure occurred |
| `close_reason` | string | Resolution description |
| `closed_by_session` | string | Session/actor that closed |

### 2.2 Dependency Entity

Dependencies represent directed relationships between issues.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `issue_id` | string | Must exist | The dependent issue |
| `depends_on_id` | string | Must exist | The issue being depended upon |
| `type` | enum | See below | Nature of the relationship |
| `created_at` | timestamp | RFC3339 | When created |
| `created_by` | string | Optional | Who created |

#### 2.2.1 Dependency Types

| Type | Semantics |
|------|-----------|
| `blocks` | `depends_on_id` must close before `issue_id` can proceed |
| `related` | Informational link, no workflow implications |
| `parent-child` | Hierarchical containment relationship |
| `discovered-from` | Tracks issue origin/derivation |
| `conditional-blocks` | Blocks only under certain conditions (metadata-dependent) |

### 2.3 Label Entity

Labels are string tags attached to issues.

| Field | Type | Description |
|-------|------|-------------|
| `issue_id` | string | The tagged issue |
| `name` | string | Label text (max 64 chars, case-insensitive for matching) |
| `created_at` | timestamp | When applied |
| `created_by` | string | Who applied |

### 2.4 Comment Entity

Comments are threaded discussions on issues.

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Unique within issue, monotonically increasing |
| `issue_id` | string | The commented issue |
| `author` | string | Comment author |
| `body` | string | Comment text (max 64KB) |
| `created_at` | timestamp | When posted |

### 2.5 Event Entity (Audit Trail)

All state changes MUST be recorded as events.

| Field | Type | Description |
|-------|------|-------------|
| `issue_id` | string | Affected issue |
| `event_type` | string | Type of change (created, updated, status-changed, commented, closed, reopened, deleted) |
| `actor` | string | Who made the change |
| `timestamp` | timestamp | When it occurred |
| `changes` | object | Field-level before/after values |
| `reason` | string | Optional context |

---

## 3. Persistence Format

### 3.1 Directory Structure

All data resides in a `.beads/` directory at the repository root:

```
.beads/
├── issues.jsonl          # Primary issue storage
├── config.yaml           # Repository configuration
├── molecules.jsonl       # Template catalog (optional)
└── hooks/                # Custom hook scripts (optional)
```

### 3.2 JSONL Format

Issues MUST be stored in newline-delimited JSON (JSONL) format in `issues.jsonl`.

#### 3.2.1 Requirements

1. One complete JSON object per line
2. No trailing commas or multi-line objects
3. UTF-8 encoding
4. Lines sorted by `id` field (lexicographic ascending) for deterministic output
5. Empty values MAY be omitted from serialization
6. Timestamps MUST use RFC3339 format with timezone

#### 3.2.2 Example

```jsonl
{"id":"bd-001","title":"Implement login","status":"closed","priority":1,"created_at":"2024-01-15T10:00:00Z","updated_at":"2024-01-16T14:30:00Z","closed_at":"2024-01-16T14:30:00Z"}
{"id":"bd-002","title":"Add dark mode","status":"open","priority":2,"created_at":"2024-01-15T11:00:00Z","updated_at":"2024-01-15T11:00:00Z","dependencies":[{"issue_id":"bd-002","depends_on_id":"bd-001","type":"blocks","created_at":"2024-01-15T11:00:00Z"}]}
```

#### 3.2.3 Embedded Collections

Dependencies, labels, and comments SHOULD be embedded within the issue object:

```json
{
  "id": "bd-003",
  "title": "Example",
  "dependencies": [{"issue_id": "bd-003", "depends_on_id": "bd-001", "type": "blocks"}],
  "labels": ["backend", "urgent"],
  "comments": [{"id": 1, "author": "alice", "body": "Started work", "created_at": "..."}]
}
```

### 3.3 ID Generation

Issue IDs MUST follow this format: `bd-<identifier>`

#### 3.3.1 Requirements

1. Prefix MUST be `bd-`
2. Identifier MUST be 3-10 lowercase alphanumeric characters
3. IDs MUST be unique within a repository
4. IDs MUST be stable (never change after creation)
5. Implementations SHOULD use content-based hashing for distributed uniqueness

#### 3.3.2 Recommended Algorithm

1. Compute SHA-256 of: `title + created_at + created_by + random_nonce`
2. Encode first 5 bytes as base32 (lowercase, no padding)
3. If collision detected, append incrementing suffix: `bd-abc12-1`, `bd-abc12-2`

---

## 4. Storage Interface

Implementations MUST provide a storage abstraction supporting these operations.

### 4.1 Issue Operations

| Operation | Signature | Description |
|-----------|-----------|-------------|
| Create | `create(issue) → id` | Create new issue, return assigned ID |
| Get | `get(id) → issue` | Retrieve issue by ID |
| Update | `update(issue) → void` | Modify existing issue |
| Delete | `delete(id, reason) → void` | Soft-delete (set tombstone status) |
| List | `list(filter) → issues[]` | Query issues with filter criteria |
| Search | `search(query) → issues[]` | Full-text search |

### 4.2 Dependency Operations

| Operation | Signature | Description |
|-----------|-----------|-------------|
| Add | `add_dependency(from, to, type) → void` | Create dependency |
| Remove | `remove_dependency(from, to) → void` | Delete dependency |
| Get Dependencies | `get_dependencies(id) → deps[]` | What does this issue depend on? |
| Get Dependents | `get_dependents(id) → deps[]` | What depends on this issue? |
| Detect Cycles | `detect_cycles() → cycles[]` | Find circular dependencies |

### 4.3 Query Operations

| Operation | Signature | Description |
|-----------|-----------|-------------|
| Get Ready Work | `get_ready_work() → issues[]` | Open issues with no open blocking dependencies |
| Get By Status | `get_by_status(status) → issues[]` | Filter by status |
| Get By Label | `get_by_label(label) → issues[]` | Filter by label |
| Get By Assignee | `get_by_assignee(assignee) → issues[]` | Filter by assignee |

### 4.4 Transaction Support

Implementations SHOULD support atomic transactions:

| Operation | Description |
|-----------|-------------|
| Begin | Start a transaction |
| Commit | Persist all changes |
| Rollback | Discard all changes |

---

## 5. Synchronization

### 5.1 Sync Modes

Implementations MUST support at least `git-portable` mode:

| Mode | Behavior |
|------|----------|
| `git-portable` | Export to JSONL before push; import from JSONL after pull |
| `realtime` | Export to JSONL on every change |

### 5.2 Export Process

1. Read all issues from storage
2. Sort by ID (ascending lexicographic)
3. Serialize each issue as single-line JSON
4. Write to `.beads/issues.jsonl`
5. Exclude fields with empty/default values

### 5.3 Import Process

1. Parse `.beads/issues.jsonl` line by line
2. For each issue:
   - If ID doesn't exist locally: create
   - If ID exists and content differs: merge (see §5.4)
   - If ID exists and content matches: skip
3. Handle issues that exist locally but not in JSONL (may have been deleted remotely)

### 5.4 Three-Way Merge Algorithm

When the same issue has been modified in multiple clones, use three-way merge:

**Inputs:**
- Base: Common ancestor version
- Left: Local version
- Right: Remote version

**Output:** Merged version

#### 5.4.1 Field-Level Merge Rules

| Field | Merge Strategy |
|-------|----------------|
| `title` | Side with latest `updated_at` wins |
| `description` | Side with latest `updated_at` wins |
| `notes` | Concatenate both changes with separator `\n\n---\n\n` |
| `status` | See §5.4.2 |
| `priority` | Lower number (higher priority) wins |
| `dependencies` | Union of additions; removals are authoritative |

#### 5.4.2 Status Merge Priority

When merging status, apply this precedence (highest first):

1. **tombstone** - Deletion is explicit and permanent
2. **closed** - Closure should not be undone by concurrent edits
3. Standard three-way merge for other statuses

#### 5.4.3 Tombstone Semantics

Tombstones (soft-deleted issues) require special handling:

1. Tombstone status wins over all other statuses
2. Tombstones have a TTL (default: 7 days)
3. After TTL expiration, a live issue MAY "resurrect" a tombstone
4. Clock skew grace period: 1 hour

```
TTL Check:
  if (now - deleted_at) > (TTL + grace_period):
    resurrection_allowed = true
```

#### 5.4.4 Conflict Resolution Strategies

Implementations SHOULD support configurable strategies:

| Strategy | Behavior |
|----------|----------|
| `newest` | Side with latest `updated_at` wins all fields |
| `ours` | Local version wins all conflicts |
| `theirs` | Remote version wins all conflicts |
| `manual` | Prompt user for each conflict |

#### 5.4.5 Complete Merge Algorithm (Pseudocode)

```
function merge_issues(base_jsonl, left_jsonl, right_jsonl) -> merged_jsonl:
    base_map = parse_to_map(base_jsonl)    // id -> issue
    left_map = parse_to_map(left_jsonl)
    right_map = parse_to_map(right_jsonl)

    all_ids = union(keys(base_map), keys(left_map), keys(right_map))
    result = []

    for id in all_ids:
        base = base_map.get(id)
        left = left_map.get(id)
        right = right_map.get(id)

        merged = merge_single_issue(base, left, right)
        if merged is not null:
            result.append(merged)

    return sort_by_id(result)

function merge_single_issue(base, left, right) -> issue or null:
    // Case 1: Only in one side (addition or deletion)
    if left is null and right is null:
        return null  // Deleted from both
    if base is null and left is null:
        return right  // Added in right only
    if base is null and right is null:
        return left   // Added in left only
    if left is null:
        return null   // Deleted in left, deletion wins
    if right is null:
        return null   // Deleted in right, deletion wins

    // Case 2: Both sides have the issue
    left_tomb = is_tombstone(left)
    right_tomb = is_tombstone(right)

    // Tombstone handling
    if left_tomb and right_tomb:
        return merge_tombstones(left, right)
    if left_tomb and not is_expired(left):
        return left  // Tombstone wins
    if right_tomb and not is_expired(right):
        return right  // Tombstone wins
    if left_tomb and is_expired(left):
        return right  // Resurrection allowed
    if right_tomb and is_expired(right):
        return left   // Resurrection allowed

    // Case 3: Standard field merge
    result = new_issue()
    result.id = left.id
    result.created_at = left.created_at
    result.created_by = left.created_by

    // Apply field-specific merge rules
    result.title = merge_by_updated_at(base.title, left, right)
    result.description = merge_by_updated_at(base.description, left, right)
    result.notes = merge_notes(base.notes, left.notes, right.notes)
    result.status = merge_status(base.status, left.status, right.status)
    result.priority = merge_priority(base.priority, left.priority, right.priority)
    result.dependencies = merge_dependencies(base.deps, left.deps, right.deps)
    result.updated_at = max(left.updated_at, right.updated_at)

    return result

function merge_status(base, left, right) -> status:
    // Priority: tombstone > closed > standard merge
    if left == "tombstone" or right == "tombstone":
        return "tombstone"
    if left == "closed" or right == "closed":
        return "closed"
    return standard_3way_merge(base, left, right)

function merge_dependencies(base, left, right) -> deps[]:
    base_set = to_set(base)
    left_set = to_set(left)
    right_set = to_set(right)

    result = []
    all_deps = union(base_set, left_set, right_set)

    for dep in all_deps:
        in_base = dep in base_set
        in_left = dep in left_set
        in_right = dep in right_set

        if in_base:
            // Was in base - check for removals
            if not in_left or not in_right:
                continue  // Removal wins
            result.append(dep)
        else:
            // Not in base - additions included
            if in_left or in_right:
                result.append(dep)

    return result

function standard_3way_merge(base, left, right) -> value:
    if base == left and base != right:
        return right  // Right changed
    if base == right and base != left:
        return left   // Left changed
    return left       // Both changed or no change - left wins
```

---

## 6. Configuration

### 6.1 Configuration Hierarchy

Settings are loaded in this order (later overrides earlier):

1. Built-in defaults
2. User config: `~/.config/beads/config.yaml` or `~/.beads/config.yaml`
3. Project config: `.beads/config.yaml`
4. Environment variables: `BEADS_*` prefix

### 6.2 Configuration Schema

```yaml
# Synchronization settings
sync:
  mode: git-portable          # git-portable | realtime
  conflict_strategy: newest   # newest | ours | theirs | manual
  auto_sync: true             # Automatically sync on git operations

# Export settings
export:
  include_closed: true        # Include closed issues in export
  include_tombstones: true    # Include tombstones (required for distributed delete)

# Compaction settings
compaction:
  enabled: false              # Enable automatic compaction
  age_threshold_days: 30      # Compact issues closed longer than this

# Identity
identity:
  name: ""                    # Default author name
  email: ""                   # Default author email
```

### 6.3 Environment Variables

| Variable | Overrides |
|----------|-----------|
| `BEADS_SYNC_MODE` | `sync.mode` |
| `BEADS_CONFLICT_STRATEGY` | `sync.conflict_strategy` |
| `BEADS_DEBUG` | Enable debug logging |

---

## 7. Command-Line Interface

### 7.1 Core Commands

| Command | Arguments | Description |
|---------|-----------|-------------|
| `init` | `[path]` | Initialize beads in a repository |
| `create` | `<title> [--desc] [--type] [--priority]` | Create new issue |
| `list` | `[--status] [--assignee] [--label]` | List issues |
| `show` | `<id>` | Display issue details |
| `update` | `<id> [--title] [--desc] [--status] ...` | Modify issue |
| `close` | `<id> [--reason]` | Close an issue |
| `reopen` | `<id>` | Reopen a closed issue |
| `delete` | `<id> [--reason]` | Soft-delete an issue |

### 7.2 Dependency Commands

| Command | Arguments | Description |
|---------|-----------|-------------|
| `dep add` | `<from-id> <to-id> [--type]` | Add dependency |
| `dep remove` | `<from-id> <to-id>` | Remove dependency |
| `dep tree` | `<id>` | Show dependency tree |
| `dep cycles` | | Detect circular dependencies |

### 7.3 Organization Commands

| Command | Arguments | Description |
|---------|-----------|-------------|
| `ready` | `[--assignee]` | List ready (unblocked) work |
| `label` | `<id> <add|remove> <label>` | Manage labels |
| `comment` | `<id> <text>` | Add comment |
| `search` | `<query>` | Full-text search |

### 7.4 Sync Commands

| Command | Arguments | Description |
|---------|-----------|-------------|
| `sync` | | Sync with git (export/import as needed) |
| `export` | `[--output]` | Export to JSONL |
| `import` | `[--input]` | Import from JSONL |

### 7.5 Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments |
| 3 | Issue not found |
| 4 | Conflict requiring resolution |

---

## 8. Background Daemon (Optional)

Implementations MAY provide a background daemon for improved performance.

### 8.1 Purpose

- Reduce startup latency for CLI commands
- Enable file watching for auto-sync
- Provide async operations without blocking CLI

### 8.2 Communication

- Unix domain socket (POSIX) or named pipe (Windows)
- JSON-based request/response protocol
- Socket path: platform temp directory + unique identifier

### 8.3 Operations

The daemon SHOULD support these RPC operations:

| Operation | Description |
|-----------|-------------|
| `ping` | Health check |
| `create_issue` | Create issue |
| `get_issue` | Retrieve issue |
| `update_issue` | Modify issue |
| `list_issues` | Query issues |
| `sync` | Trigger sync |
| `shutdown` | Graceful shutdown |

### 8.4 Lifecycle

1. CLI checks for running daemon via socket
2. If not running and `auto_daemon: true`, CLI starts daemon
3. Daemon exits after idle timeout (default: 30 minutes)
4. Daemon writes PID file for process management

### 8.5 RPC Protocol

**Message Format:**

Request:
```json
{
  "id": "unique-request-id",
  "method": "create_issue",
  "params": {
    "title": "Issue title",
    "description": "Description",
    "priority": 1
  }
}
```

Response (success):
```json
{
  "id": "unique-request-id",
  "result": {
    "issue_id": "bd-abc12"
  }
}
```

Response (error):
```json
{
  "id": "unique-request-id",
  "error": {
    "code": 404,
    "message": "Issue not found"
  }
}
```

**Error Codes:**
| Code | Meaning |
|------|---------|
| 400 | Invalid request |
| 404 | Not found |
| 409 | Conflict |
| 500 | Internal error |

**Connection Protocol:**
1. Client connects to socket
2. Send newline-delimited JSON requests
3. Read newline-delimited JSON responses
4. Match responses to requests by `id` field
5. Close connection or keep alive for multiple requests

### 8.6 File Watching

The daemon SHOULD watch for changes to:
- `.beads/issues.jsonl` - Trigger import on external modification
- `.beads/config.yaml` - Reload configuration

**Debouncing:** Wait 100-500ms after last change before processing to batch rapid edits.

---

## 9. Hooks

### 9.1 Git Hooks

Implementations SHOULD integrate with these git hooks:

| Hook | Action |
|------|--------|
| `post-commit` | Export JSONL |
| `post-merge` | Import JSONL |
| `post-checkout` | Import JSONL (if branch changed) |
| `pre-push` | Export JSONL, commit if changed |

### 9.2 Custom Hooks

Scripts in `.beads/hooks/` are executed at defined points:

| Hook | Arguments | When |
|------|-----------|------|
| `post-create` | `<issue-id>` | After issue creation |
| `post-update` | `<issue-id>` | After issue modification |
| `post-close` | `<issue-id>` | After issue closure |
| `pre-sync` | | Before sync operation |
| `post-sync` | | After sync operation |

---

## 10. Templates (Molecules)

### 10.1 Purpose

Molecules are reusable templates for common work patterns.

### 10.2 Storage

Templates stored in `.beads/molecules.jsonl` with `is_template: true` field.

### 10.3 Template Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Template identifier (prefix: `mol-`) |
| `title` | string | Template name |
| `is_template` | boolean | Must be `true` |
| `steps` | array | Sequence of issue templates |
| `variables` | object | Variable definitions with defaults |

### 10.4 Variable Substitution

Templates support variable substitution using `{{variable_name}}` syntax:

```yaml
title: "Review PR for {{feature_name}}"
assignee: "{{reviewer}}"
```

### 10.5 Instantiation

When a template is instantiated:
1. Deep copy the template
2. Generate new unique IDs for all issues
3. Substitute all variables with provided values
4. Remove `is_template` flag
5. Create issues and dependencies

---

## 11. Compaction

### 11.1 Purpose

Reduce storage size and AI context window usage by summarizing old completed work.

### 11.2 Eligibility

An issue is eligible for compaction when:
- Status is `closed`
- `closed_at` is older than threshold (default: 30 days)
- No open issues depend on it

### 11.3 Process

1. Generate summary from title, description, and close_reason
2. Set `compaction_level` field (1 = summarized, 2 = archived)
3. Truncate description/notes to summary
4. Preserve: id, title (may shorten), status, key metadata
5. Remove: full description, comments, design docs

### 11.4 Compacted Issue Fields

| Field | Behavior |
|-------|----------|
| `compaction_level` | Set to compaction level |
| `compacted_at` | Timestamp of compaction |
| `original_size` | Byte count before compaction |
| `summary` | AI-generated or extracted summary |

---

## 12. Multi-Repository Federation (Optional)

### 12.1 Routing

Issues can be routed to different repositories based on ID prefix:

```yaml
routing:
  rules:
    - prefix: "frontend-"
      repository: "org/frontend-repo"
    - prefix: "backend-"
      repository: "org/backend-repo"
    - prefix: "*"
      repository: "org/main-repo"
```

### 12.2 Cross-Repository Dependencies

Dependencies MAY reference issues in other repositories using full identifiers:
- Format: `<repository>:<issue-id>`
- Example: `org/frontend-repo:bd-abc12`

### 12.3 Sovereignty Tiers

| Tier | Permissions |
|------|-------------|
| T1 (Full) | Complete read/write control |
| T2 (Shared) | Read/write with merge required |
| T3 (Delegated) | Write requires approval |
| T4 (Read-only) | Read access only |

---

## 13. Invariants and Constraints

### 13.1 Data Integrity

1. Every issue MUST have a unique, non-empty `id`
2. Every issue MUST have a non-empty `title`
3. `updated_at` MUST be >= `created_at`
4. `closed_at` MUST only be set when `status` = `closed`
5. Tombstone fields MUST only be set when `status` = `tombstone`
6. Dependencies MUST NOT create cycles (enforced at creation time)
7. Dependencies MUST reference existing issues

### 13.2 Behavioral Guarantees

1. Deleting an issue MUST NOT orphan dependencies (cascade to tombstone or reject)
2. Closing an issue MUST NOT automatically close dependents
3. Export MUST be deterministic (same data = same output)
4. Import MUST be idempotent (repeated import = no change)

### 13.3 Ordering Guarantees

1. Events for an issue MUST be ordered by timestamp
2. Comments MUST be ordered by ID (monotonically increasing)
3. JSONL export MUST be sorted by issue ID

---

## 14. Error Handling

### 14.1 Error Categories

| Category | Recovery |
|----------|----------|
| Not Found | Return appropriate error, do not create |
| Conflict | Apply conflict resolution strategy |
| Validation | Reject with specific field errors |
| Storage | Retry with backoff, then fail |
| Network | Retry with backoff, then operate offline |

### 14.2 Partial Failure

During bulk operations:
- Continue processing remaining items
- Collect all errors
- Report summary at end
- Rollback if transactional

---

## Appendix A: JSONL Schema (JSON Schema)

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["id", "title", "status", "created_at", "updated_at"],
  "properties": {
    "id": {
      "type": "string",
      "pattern": "^bd-[a-z0-9]{3,10}(-[0-9]+)?$"
    },
    "title": {
      "type": "string",
      "minLength": 1,
      "maxLength": 500
    },
    "status": {
      "type": "string",
      "enum": ["open", "in-progress", "blocked", "deferred", "closed", "tombstone"]
    },
    "priority": {
      "type": "integer",
      "minimum": 0,
      "maximum": 4,
      "default": 2
    },
    "created_at": {
      "type": "string",
      "format": "date-time"
    },
    "updated_at": {
      "type": "string",
      "format": "date-time"
    },
    "dependencies": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["issue_id", "depends_on_id", "type"],
        "properties": {
          "issue_id": {"type": "string"},
          "depends_on_id": {"type": "string"},
          "type": {
            "type": "string",
            "enum": ["blocks", "related", "parent-child", "discovered-from", "conditional-blocks"]
          }
        }
      }
    },
    "labels": {
      "type": "array",
      "items": {"type": "string", "maxLength": 64}
    },
    "comments": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["id", "author", "body", "created_at"],
        "properties": {
          "id": {"type": "integer"},
          "author": {"type": "string"},
          "body": {"type": "string"},
          "created_at": {"type": "string", "format": "date-time"}
        }
      }
    }
  }
}
```

---

## Appendix B: Example Session

```bash
# Initialize in a git repository
$ bd init
Initialized beads in .beads/

# Create issues
$ bd create "Implement user authentication" --type feature --priority 1
Created bd-a1b2c

$ bd create "Write login form" --type task
Created bd-d3e4f

# Add dependency
$ bd dep add bd-d3e4f bd-a1b2c --type blocks
Added: bd-d3e4f blocks bd-a1b2c

# List ready work
$ bd ready
bd-d3e4f  Write login form  [open]  P2

# Update and close
$ bd update bd-d3e4f --status in-progress --assignee alice
Updated bd-d3e4f

$ bd close bd-d3e4f --reason "Completed and tested"
Closed bd-d3e4f

# Sync with git
$ bd sync
Exported 2 issues to .beads/issues.jsonl

$ git add .beads/ && git commit -m "Update issues"
$ git push
```

---

## 15. Agent Coordination System

This section specifies features for AI agent coordination in multi-agent environments.

### 15.1 Agent Identity Fields

Issues representing agents SHOULD include these fields:

| Field | Type | Description |
|-------|------|-------------|
| `agent_state` | enum | Current state: idle, spawning, running, working, stuck, done, stopped, dead |
| `last_activity` | timestamp | Last heartbeat/action timestamp for timeout detection |
| `role_type` | string | Agent role: polecat, crew, witness, refinery, mayor, deacon |
| `rig` | string | Workspace/repository context identifier |

### 15.2 Slot Mechanism

Slots enforce cardinality constraints on issue references (0..1 relationship):

| Field | Type | Description |
|-------|------|-------------|
| `hook_bead` | string | Currently assigned work (at most one) |
| `role_bead` | string | Reference to role definition |

**Operations:**
- `set_slot(agent_id, slot_name, bead_id)` - Assign (fails if occupied)
- `clear_slot(agent_id, slot_name)` - Release
- `get_slot(agent_id, slot_name)` - Query current value

### 15.3 Gate Mechanism

Gates provide async coordination primitives for waiting on external conditions:

| Field | Type | Description |
|-------|------|-------------|
| `await_type` | string | Condition type: timer, human, bead, or platform-specific |
| `await_id` | string | Condition identifier (e.g., workflow ID, PR number) |
| `timeout` | duration | Maximum wait time before escalation |
| `waiters` | string[] | Entities to notify when gate resolves |

**Gate Types:**
| Type | Resolution Condition |
|------|---------------------|
| `timer` | Timeout duration elapsed |
| `human` | Manual resolution via command |
| `bead` | Referenced issue closes |

**Operations:**
- `create_gate(issue_id, type, id, timeout)` - Create gate
- `check_gates()` - Evaluate all gates, auto-resolve if conditions met
- `resolve_gate(issue_id)` - Manual resolution
- `add_waiter(issue_id, waiter)` - Register notification target

### 15.4 Messaging Fields

For inter-agent communication:

| Field | Type | Description |
|-------|------|-------------|
| `sender` | string | Message originator identifier |
| `ephemeral` | boolean | If true, excluded from JSONL export; can be bulk-deleted |

### 15.5 Session Tracking

| Field | Type | Description |
|-------|------|-------------|
| `closed_by_session` | string | Session identifier that closed the issue |

### 15.6 Agent State Machine

```
idle → spawning → running ↔ working → done → stopped
                     ↓         ↓
                   stuck     dead
```

| State | Description |
|-------|-------------|
| `idle` | Waiting for work assignment |
| `spawning` | Starting up |
| `running` | Executing (general) |
| `working` | Actively working on specific task |
| `stuck` | Blocked, needs intervention |
| `done` | Completed current work |
| `stopped` | Clean shutdown |
| `dead` | Unclean termination (detected via timeout) |

---

## 16. Advanced Synchronization Features

### 16.1 Export Policies

Implementations SHOULD support configurable error handling during export:

| Policy | Behavior |
|--------|----------|
| `strict` | Fail immediately on any error (default for manual export) |
| `best-effort` | Skip failures with warnings, continue processing (default for auto-export) |
| `partial` | Retry transient failures with backoff; skip persistent failures |
| `required-core` | Fail on issue/dependency errors; best-effort for labels/comments |

**Configuration:**
```yaml
export:
  error_policy: strict
  retry_attempts: 3
  retry_backoff_ms: 100
```

### 16.2 Additional Sync Modes

Beyond the required `git-portable` mode:

| Mode | Behavior |
|------|----------|
| `dolt-native` | Use Dolt database remotes directly; skip JSONL |
| `belt-and-suspenders` | Use both Dolt remotes AND JSONL for redundancy |

### 16.3 Sync Branch Mode

For team workflows, a dedicated sync branch prevents conflicts:

**Requirements:**
- Branch name MUST NOT be `main` or `master`
- Branch name MUST match pattern: `^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$`
- No consecutive dots (`..`)
- Maximum 255 characters

**Configuration:**
```yaml
sync:
  branch: beads-sync
```

### 16.4 Dirty Tracking

For incremental exports:

**Operations:**
- `mark_dirty(issue_id)` - Flag issue as modified since last export
- `get_dirty_issues()` - Return IDs of modified issues
- `clear_dirty(issue_ids)` - Clear flags after successful export
- `get_export_hash()` - Content hash of last export
- `set_export_hash(hash)` - Store hash after export

**Algorithm:**
```
if incremental_export:
    dirty_ids = get_dirty_issues()
    existing_issues = parse_jsonl(issues.jsonl)
    for id in dirty_ids:
        existing_issues[id] = get_issue(id)
    write_jsonl(existing_issues)
    clear_dirty(dirty_ids)
```

### 16.5 Import Validation

**Collision Detection:**
| Category | Condition | Action |
|----------|-----------|--------|
| Exact Match | Same ID and content | Skip (idempotent) |
| Collision | Same ID, different content | Merge per strategy |
| New | ID doesn't exist | Create |

**Orphan Handling:**
| Mode | Behavior |
|------|----------|
| `strict` | Fail if parent issue missing |
| `skip` | Skip orphaned issues with warning |
| `allow` | Import orphans without validation (default) |

---

## 17. Formula System

Formulas define reusable workflow templates with composition capabilities.

### 17.1 Formula Types

| Type | Purpose |
|------|---------|
| `workflow` | Standard multi-step process template |
| `expansion` | Macro that expands into multiple steps |
| `aspect` | Cross-cutting concern applied to other formulas |

### 17.2 Formula Definition

```yaml
formula: mol-feature
description: Standard feature development workflow
version: 1
type: workflow
extends: []
vars:
  feature_name:
    description: Name of the feature
    required: true
  reviewer:
    description: Code reviewer
    default: ""
steps:
  - id: design
    title: "Design {{feature_name}}"
    type: task
    priority: 1
  - id: implement
    title: "Implement {{feature_name}}"
    depends_on: [design]
  - id: review
    title: "Review {{feature_name}}"
    depends_on: [implement]
    assignee: "{{reviewer}}"
```

### 17.3 Variable Substitution

**Syntax:** `{{variable_name}}`

**Scope:** Variables are substituted at instantiation time.

**Validation:**
- `required: true` - Must be provided
- `enum: [a, b, c]` - Value must be in list
- `pattern: "^[a-z]+$"` - Must match regex

### 17.4 Inheritance

Formulas can extend other formulas:

```yaml
formula: mol-secure-feature
extends: [mol-feature]
steps:
  - id: security-review
    title: "Security review for {{feature_name}}"
    depends_on: [implement]
```

**Resolution:** Parent definitions loaded first; child overrides on conflict.

### 17.5 Composition Rules

**Bond Points:** Named attachment sites for extending workflows
```yaml
compose:
  bond_points:
    - id: after-design
      after_step: design
```

**Expansions:** Replace steps with expanded templates
```yaml
compose:
  expand:
    - target: implement
      with: exp-tdd-cycle
      vars:
        coverage: "80"
```

**Aspects:** Apply cross-cutting concerns
```yaml
compose:
  aspects: [security-audit, logging]
```

### 17.6 Aspect Advice

Aspects use advice rules to inject steps:

```yaml
formula: asp-security
type: aspect
advice:
  - target: "*.implement"  # Glob pattern
    after:
      id: "security-scan-{step.id}"
      title: "Security scan after {step.title}"
```

**Target Patterns:**
- `"design"` - Exact match
- `"*.implement"` - Suffix match
- `"feature.*"` - Prefix match
- `"*"` - Match all

### 17.7 Step Definition

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique within formula |
| `title` | string | Issue title (supports variables) |
| `description` | string | Issue description |
| `type` | string | task, bug, feature, epic |
| `priority` | integer | 0-4 |
| `depends_on` | string[] | Step ID dependencies |
| `assignee` | string | Default assignee |
| `condition` | string | Include only if condition true |
| `expand` | string | Inline expansion formula |

### 17.8 Control Flow

**Loops:**
```yaml
- id: iteration
  loop:
    count: 3  # Fixed count
    # OR
    range: "1..{{max_iterations}}"  # Variable range
```

**Gates:**
```yaml
- id: wait-for-approval
  gate:
    type: human
    timeout: 24h
```

**Conditions:**
```yaml
- id: optional-step
  condition: "{{include_tests}} == true"
```

### 17.9 Instantiation Process

1. Load formula and resolve inheritance chain
2. Apply composition rules (expansions, aspects)
3. Filter steps by conditions
4. Substitute variables
5. Create issues with dependencies

---

## 18. Ready Work Algorithm

### 18.1 Definition

An issue is "ready" when:
1. Status is `open`
2. No blocking dependencies exist where the blocker is not closed

### 18.2 Algorithm

```
function get_ready_work():
    ready = []
    for issue in get_issues_by_status("open"):
        if is_ready(issue):
            ready.append(issue)
    return sort_by_priority(ready)

function is_ready(issue):
    for dep in get_dependencies(issue.id):
        if dep.type in [blocks, parent-child, conditional-blocks]:
            blocker = get_issue(dep.depends_on_id)
            if blocker.status not in [closed, tombstone]:
                return false
    return true
```

### 18.3 Sorting

Ready issues SHOULD be sorted by:
1. Priority (ascending: 0 = highest)
2. Created date (ascending: oldest first)
3. ID (lexicographic, for determinism)

---

## 19. Multi-Process Coordination

### 19.1 Lock File Protocol

For coordinating multiple CLI processes:

**Lock File Location:** `.beads/.lock`

**Protocol:**
1. Acquire lock before write operations
2. Use advisory locking (flock on POSIX, LockFileEx on Windows)
3. Release lock after operation completes
4. Timeout: 30 seconds default

### 19.2 Database Locking

For SQL-based storage:
- Use IMMEDIATE transactions for writes
- Allows concurrent readers during write transaction
- Prevents write-write conflicts

### 19.3 Daemon Coordination

If daemon is running:
- CLI connects via socket before operations
- Daemon holds primary database connection
- CLI operations route through daemon
- File watcher notifies daemon of external changes

### 19.4 Export Atomicity

To prevent partial exports:
1. Write to temporary file
2. Compute content hash
3. Atomic rename to target path
4. Update export metadata

---

## 20. Extended Dependency Types

Beyond basic blocking relationships:

| Type | Semantics | Affects Ready? |
|------|-----------|----------------|
| `blocks` | Must close before dependent proceeds | Yes |
| `parent-child` | Hierarchical containment | Yes |
| `conditional-blocks` | Blocks only if condition met | Yes |
| `waits-for` | Fanout gate for dynamic children | Yes |
| `related` | Informational link | No |
| `discovered-from` | Origin tracking | No |
| `caused-by` | Audit trail | No |
| `tracks` | Cross-project reference | No |

---

## Appendix C: Recommended Database Schema

For implementations using SQL storage:

```sql
-- Core issues table
CREATE TABLE issues (
    id TEXT PRIMARY KEY,
    content_hash TEXT,
    title TEXT NOT NULL,
    description TEXT,
    design TEXT,
    acceptance_criteria TEXT,
    notes TEXT,
    status TEXT NOT NULL DEFAULT 'open',
    priority INTEGER DEFAULT 2,
    issue_type TEXT DEFAULT 'task',
    assignee TEXT,
    owner TEXT,
    estimated_minutes INTEGER,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    closed_at TEXT,
    close_reason TEXT,
    closed_by_session TEXT,
    due_at TEXT,
    defer_until TEXT,
    external_ref TEXT,
    source_system TEXT,
    created_by TEXT,
    -- Tombstone fields
    deleted_at TEXT,
    deleted_by TEXT,
    delete_reason TEXT,
    original_type TEXT,
    -- Agent fields
    hook_bead TEXT,
    role_bead TEXT,
    agent_state TEXT,
    last_activity TEXT,
    role_type TEXT,
    rig TEXT,
    -- Gate fields
    await_type TEXT,
    await_id TEXT,
    timeout_ns INTEGER,
    waiters TEXT,  -- JSON array
    -- Messaging fields
    sender TEXT,
    ephemeral INTEGER DEFAULT 0,
    -- Compaction fields
    compaction_level INTEGER DEFAULT 0,
    compacted_at TEXT,
    original_size INTEGER,
    summary TEXT,
    -- Formula tracking
    source_formula TEXT,
    source_location TEXT
);

-- Indexes for common queries
CREATE INDEX idx_issues_status ON issues(status);
CREATE INDEX idx_issues_assignee ON issues(assignee);
CREATE INDEX idx_issues_priority ON issues(priority);
CREATE INDEX idx_issues_updated_at ON issues(updated_at);
CREATE INDEX idx_issues_external_ref ON issues(external_ref);
CREATE INDEX idx_issues_ephemeral ON issues(ephemeral) WHERE ephemeral = 1;

-- Dependencies table
CREATE TABLE dependencies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id TEXT NOT NULL,
    depends_on_id TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'blocks',
    created_at TEXT,
    created_by TEXT,
    metadata TEXT,  -- JSON for type-specific data
    FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE,
    FOREIGN KEY (depends_on_id) REFERENCES issues(id) ON DELETE CASCADE,
    UNIQUE(issue_id, depends_on_id, type)
);

CREATE INDEX idx_dependencies_issue ON dependencies(issue_id);
CREATE INDEX idx_dependencies_depends_on ON dependencies(depends_on_id);

-- Labels table
CREATE TABLE labels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TEXT,
    created_by TEXT,
    FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE,
    UNIQUE(issue_id, name)
);

CREATE INDEX idx_labels_issue ON labels(issue_id);
CREATE INDEX idx_labels_name ON labels(name);

-- Comments table
CREATE TABLE comments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id TEXT NOT NULL,
    author TEXT,
    body TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE
);

CREATE INDEX idx_comments_issue ON comments(issue_id);

-- Events table (audit trail)
CREATE TABLE events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    actor TEXT,
    timestamp TEXT NOT NULL,
    changes TEXT,  -- JSON object
    reason TEXT,
    FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE
);

CREATE INDEX idx_events_issue ON events(issue_id);
CREATE INDEX idx_events_timestamp ON events(timestamp);

-- Export tracking for dirty/incremental exports
CREATE TABLE export_metadata (
    key TEXT PRIMARY KEY,
    value TEXT,
    updated_at TEXT
);

-- Dirty issues for incremental export
CREATE TABLE dirty_issues (
    issue_id TEXT PRIMARY KEY,
    marked_at TEXT NOT NULL,
    FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE
);

-- Configuration storage
CREATE TABLE config (
    key TEXT PRIMARY KEY,
    value TEXT,
    updated_at TEXT
);
```

**Notes:**
- Use TEXT for timestamps (RFC3339 format) for portability
- JSON fields stored as TEXT for flexibility
- Enable WAL mode for concurrent access: `PRAGMA journal_mode=WAL`
- Use IMMEDIATE transactions for write operations

---

## Appendix D: Glossary

| Term | Definition |
|------|------------|
| **Advice** | Step transformation rule in aspect formulas |
| **Aspect** | Cross-cutting formula that modifies other formulas |
| **Base** | Common ancestor in three-way merge |
| **Blocking** | A dependency that prevents work from starting |
| **Bond Point** | Named attachment site in a formula |
| **Clean Room** | Implementation without reference to existing code |
| **Deterministic** | Same input always produces same output |
| **Dirty Tracking** | Marking modified issues for incremental export |
| **Ephemeral** | Issue excluded from export (temporary) |
| **Expansion** | Macro formula that expands into multiple steps |
| **Formula** | Reusable workflow template |
| **Gate** | Async coordination primitive for waiting |
| **Idempotent** | Operation can be repeated without changing result |
| **JSONL** | JSON Lines - newline-delimited JSON format |
| **Molecule** | Instantiated formula (working copy) |
| **Ready Work** | Issues with status=open and no open blocking dependencies |
| **Resurrection** | Recreating a tombstoned issue |
| **Rig** | Workspace/repository context for agents |
| **Slot** | Cardinality-enforced reference field (0..1) |
| **Tombstone** | Soft-delete marker for distributed consistency |
| **TTL** | Time To Live - expiration duration |
| **Waiter** | Entity registered for gate resolution notification |
