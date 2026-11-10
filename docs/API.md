# SiteTrack API

The default local origin is `http://127.0.0.1:8080`. All application routes start with `/v1`. Send `Content-Type: application/json` with JSON bodies and `Authorization: Bearer <access_token>` on authenticated requests.

## Conventions

- Project and task IDs are strings such as `PRJ-0001` and `PRJ-0001-T-0001`. The four-digit padding expands as needed. Other IDs are positive JSON integers.
- Dates use `YYYY-MM-DD`. Timestamps use RFC 3339. Business date comparisons use `Asia/Muscat` by default.
- Inventory uses decimal strings with at most three fractional digits on input and exactly three on output, such as `"12.500"`. Maximum quantity is `"9223372036854775.807"`. Each delivery line and supplier receipt must be positive. Reorder points can be zero.
- List routes accept `limit` from 1 to 100 and default to 50. Responses contain `items` and optionally `next_cursor`. Send `cursor=<next_cursor>` with the same filters and sort to request another page. Treat the cursor as opaque. Missing `next_cursor` marks the end. Empty lists return `"items":[]`. Lists reflect current data, so concurrent edits to a sort key can move an item across pages.
- Query values are single-valued. Unrecognized list parameters, sort names, enum values and malformed filters return 400. Boolean filters take `true` or `false`. Searches match literal case-insensitive substrings.
- Successful creates return 201. Reads, replacements and actions with a response body return 200. Successful deletes and explicit acknowledgments return 204 with an empty body.

Application errors use the following shape.

```json
{"error":{"code":"not_found","message":"resource not found"}}
```

| HTTP | Code | When |
| --- | --- | --- |
| 400 | `invalid_input` | Malformed input, unsupported filter, or invalid cursor |
| 401 | `unauthorized` | Invalid credentials, access token, or refresh token |
| 403 | `forbidden` | Authenticated actor lacks permission for the requested operation |
| 404 | `not_found` | Missing resource or inaccessible project resource |
| 409 | `conflict` | Duplicate pending invitation, illegal transition, insufficient stock, or resource protected by history |
| 500 | `internal_error` | Unexpected server error |

## Sessions and profile

| Method and path | Body | Response |
| --- | --- | --- |
| `POST /v1/auth/register` | `{name,email,password,role}` | User, 201 |
| `POST /v1/auth/login` | `{email,password}` | Token pair |
| `POST /v1/auth/refresh` | `{refresh_token}` | Replacement token pair |
| `POST /v1/auth/logout` | Empty, bearer required | 204 |
| `GET /v1/me` | Bearer required | User |
| `GET /healthz` | Public | `{"status":"ok"}` |
| `GET /readyz` | Public | 200 when PostgreSQL responds, otherwise 503 |

A user has `id`, `name`, `email`, `role`, and `created_at`. Email is trimmed and case-normalized. Passwords must contain 8–72 bytes. Role codes are `user`, `company_store_manager`, and `superadmin`.

```json
{"access_token":"<JWT>","refresh_token":"<opaque token>","expires_in":900}
```

`expires_in` is the access lifetime in seconds. Login starts a new session. Store the new refresh token after each refresh and serialize refresh requests within a session. A refresh token can be used once. Logout invalidates both tokens for that session immediately. Role and membership changes take effect on subsequent requests using an existing token.

| Actor | Access |
| --- | --- |
| Any authenticated account | Own profile, own invitations, shared materials catalogue, create a project |
| Project member | Read the project, members and pending invitations, work with tasks and dependencies, view project stock and act on deliveries as the project store |
| Project admin member | Member permissions plus edit/delete the project, invite, withdraw, change member admin status and remove members |
| Company store manager | Normal membership rules plus all company-store operations and company deliveries across projects |
| SuperAdmin | Read and use all project task/store/delivery screens and the company store. Project-admin operations require project-admin membership |

## Projects

| Method and path | Body | Response |
| --- | --- | --- |
| `GET /v1/projects` | Empty | Paginated projects |
| `POST /v1/projects` | `{name,start_date,end_date}` | Project, 201 |
| `GET /v1/projects/{projectID}` | Empty | Project |
| `PUT /v1/projects/{projectID}` | `{name,start_date,end_date}` | Project |
| `DELETE /v1/projects/{projectID}` | Empty | 204 |

The project list supports `q` on name or ID, `late_only=true`, and `sort=name|start_date|percent_done|late_tasks`. Default sort is `name` with `order=asc`. Use `order=desc` to reverse it. ID allocation order breaks ties. Creating a project creates its store and makes the creator a project admin. A project with stock or delivery history returns 409 on deletion.

```json
{
  "id":"PRJ-0001",
  "name":"Harbour utilities",
  "start_date":"2026-08-01",
  "end_date":"2026-12-15",
  "is_admin":true,
  "member_count":8,
  "task_counts":{"not_started":12,"in_progress":4,"done":8},
  "percent_done":33.33,
  "late_tasks":6,
  "blocked_tasks":5
}
```

An empty project reports zero task counts and zero percent done. Late and blocked counts may include the same task. `is_admin` describes the caller's project membership.

## Members, invitations and notifications

| Method and path | Body | Response |
| --- | --- | --- |
| `GET /v1/projects/{projectID}/members` | Empty | Paginated members in user ID order |
| `PUT /v1/projects/{projectID}/members/{userID}` | `{"is_admin":true}` or false | 204 |
| `DELETE /v1/projects/{projectID}/members/{userID}` | Empty | 204 |
| `GET /v1/projects/{projectID}/invitations` | Empty | Paginated pending invitations, newest first |
| `POST /v1/projects/{projectID}/invitations` | `{"email":"person@example.com"}` | Invitation, 201 |
| `DELETE /v1/projects/{projectID}/invitations/{invitationID}` | Empty | Withdraws a pending invitation, 204 |
| `GET /v1/notifications` | Empty | Paginated invitations for the caller, newest first, with `unread_count` |
| `GET /v1/notifications/unread-count` | Empty | `{"unread_count":2}` for the navigation badge |
| `POST /v1/notifications/{invitationID}/read` | Empty | Marks the invitation read, 204 |
| `POST /v1/notifications/{invitationID}/respond` | `{"response":"accept"}` or `{"response":"reject"}` | Updated invitation |

Member fields are `user_id`, `name`, `email`, `is_admin`, and `joined_at`. Admin changes and removals that would leave zero project admins return 409.

Invitation fields are `id`, `project_id`, `project_name`, `recipient_id`, `recipient_name`, `recipient_email`, `sender_id`, `sender_name`, `status`, `created_at`, nullable `read_at`, and nullable `responded_at`. Only the recipient can respond to a pending invitation. Accepting creates membership and both responses mark the notification read. Withdrawn invitations disappear from the recipient's inbox. The other visible statuses are `pending`, `accepted`, and `rejected`.

`project_name` is the project label saved when the invitation was sent. Later renames do not change it. For older invitations that had no stored label, it is the name at upgrade time if the invite was pending or the recipient had project access then; otherwise it is the project ID.

## Tasks and dependencies

Let `/tasks` below stand for `/v1/projects/{projectID}/tasks`.

| Method and path | Body | Response |
| --- | --- | --- |
| `GET /tasks` | Empty | Paginated tasks |
| `POST /tasks` | `{name,start_date,end_date}` | Task detail, 201 |
| `GET /tasks/{taskID}` | Empty | Task detail |
| `PUT /tasks/{taskID}` | `{name,start_date,end_date}` | Updated task detail |
| `DELETE /tasks/{taskID}` | Empty | 204 |
| `POST /tasks/{taskID}/actions/{action}` | Empty | Updated task detail |
| `POST /tasks/{taskID}/dependencies` | `{prerequisite_id}` | Prerequisite reference, 201 |
| `GET /tasks/{taskID}/dependencies/{prerequisiteID}` | Empty | Direct prerequisite reference |
| `PUT /tasks/{taskID}/dependencies/{prerequisiteID}` | `{prerequisite_id}` | Replacement prerequisite reference |
| `DELETE /tasks/{taskID}/dependencies/{prerequisiteID}` | Empty | 204 |

Task list filters are `q` on name or ID, `status=not_started|in_progress|done`, `late=true|false`, `blocked=true|false`, and `from` and `to` for inclusive planned date overlap. Either range bound can be omitted. Sort by `id`, `name`, `start_date`, `end_date`, or `dependent_count` with `order=asc|desc`. Default is ascending numeric task ID.

A task includes `id`, `name`, `status`, `start_date`, `end_date`, `late`, `blocked`, `direct_dependencies`, and `dependent_count`. The direct dependency array contains `{id,name,status}` and the count covers tasks directly waiting on this task. A detail adds `prerequisites` and `dependents` arrays with `{id,name,status,distance}`, sorted by shortest edge distance and then numeric ID. It also includes `allowed_actions`, an array with zero or one of `start`, `done`, and `reopen`.

New tasks start at `not_started`. Start requires every prerequisite to be Done. Only In progress can move to Done. Reopen requires every transitive dependent to be Not started. Late applies when a task is unfinished and its planned end date is before today, or when a task has yet to start and its planned start date is before today. Blocked applies when a task is unfinished and any of its prerequisites is unfinished. Both flags are computed on read.

Dependencies are within the same project. Cycles and self-links are invalid. An already-started task can gain a dependency only if the prerequisite is Done. Removing a task referenced by another task requires unlinking that dependency first.

## Materials and stock

| Method and path | Body | Response |
| --- | --- | --- |
| `GET /v1/materials` | Empty | Paginated materials |
| `POST /v1/materials` | `{name,category,unit}` | Material, 201 |
| `GET /v1/materials/{materialID}` | Empty | Material |
| `PATCH /v1/materials/{materialID}` | At least one of `name`, `category`, `unit` | Updated material |
| `DELETE /v1/materials/{materialID}` | Empty | 204 |
| `GET /v1/projects/{projectID}/store` | Empty | Paginated project stock |
| `GET /v1/company/store` | Empty | Paginated company stock |
| `POST /v1/company/store/{materialID}/add-stock` | `{"quantity":"12.500"}` | Updated company stock item |
| `PUT /v1/company/store/{materialID}/reorder-point` | `{"reorder_point":"5.000"}` | Updated company stock item |

Materials contain `id`, `name`, `category` and `unit`. The catalogue and stock lists accept `search` on name, `category` as a case-insensitive exact match, and `order=asc|desc`. Default sort is ascending name.

| List | `sort` values | Extra filters | Item fields in addition to material |
| --- | --- | --- | --- |
| Materials | `name` | — | — |
| Project stock | `name`, `quantity`, `last_delivery` | — | `quantity`, nullable `last_delivery_at` |
| Company stock | `name`, `quantity`, `status` | `status=out_of_stock|low_stock|available` | `quantity`, `in_transit_quantity`, `reorder_point`, `status` |

Project stock shows materials handled by that store, including those now at zero. Last delivery records the latest outgoing dispatch or successful incoming record. Unknown last-delivery timestamps sort last. The company list includes every catalogue material. Company stock is out of stock at zero, low at a positive quantity at or below its reorder point, and available above it. In-transit quantity is the material reserved for outgoing company deliveries. Ascending status sort follows out of stock, low stock, available.

Company stock changes require company-store access. Materials with inventory, reorder points, or delivery history retain their unit and cannot be deleted. Names and categories can be edited. Existing delivery lines retain their original descriptions.

## Deliveries

Use `/v1/company/deliveries` for the company store and `/v1/projects/{projectID}/deliveries` for a project store. In the following table, `/deliveries` stands for the appropriate full path.

| Method and path | Body | Response |
| --- | --- | --- |
| `GET /deliveries` | Empty | Paginated deliveries, newest first |
| `POST /deliveries` | `{project_id,lines}` on company route, `{lines}` on project route | Delivery, 201 |
| `GET /deliveries/{deliveryID}` | Empty | Delivery |
| `POST /deliveries/{deliveryID}/record` | Empty, receiver only | Delivery |
| `POST /deliveries/{deliveryID}/reject` | Empty, receiver only | Delivery |
| `POST /deliveries/{deliveryID}/cancel` | Empty, sender only | Delivery |

Initiation accepts 1–100 lines of `{material_id,quantity}` with distinct materials. The route determines which store acts and which direction the delivery travels. Initiation deducts all lines from the sender atomically. Recording credits the receiver. Rejecting or cancelling refunds the sender. Only In transit deliveries can be resolved.

Both lists accept `direction=company_to_project|project_to_company`, `status=in_transit|delivered|rejected|cancelled`, and inclusive `start_date` and `end_date` filters on the sent business date. Use `project_id` only on the company delivery list. Lists sort by newest sent timestamp, then highest delivery ID.

```json
{
  "id":901,
  "project":{"id":"PRJ-0001","name":"Harbour utilities"},
  "direction":"company_to_project",
  "status":"in_transit",
  "lines":[{"material_id":45,"name":"OPC cement","category":"Cement","unit":"bag","quantity":"12.500"}],
  "sent_at":"2026-11-10T08:00:00Z",
  "sent_by":{"id":2,"name":"Rashid Hassan"},
  "received_at":null,
  "received_by":null,
  "resolved_at":null,
  "resolved_by":null,
  "allowed_actions":["record","reject"]
}
```

The example is from the receiving project store. Actions are relative to the route's acting store. The company route for this delivery instead offers `cancel`. A terminal delivery returns an empty action array. `received_at` and `received_by` are populated only on recording. `resolved_at` and `resolved_by` describe any terminal action.
