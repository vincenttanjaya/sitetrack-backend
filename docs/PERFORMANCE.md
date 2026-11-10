# Full dataset verification

The full preset was loaded into an isolated PostgreSQL 18.6 database on Docker Desktop with `--anchor 2026-11-10`. The load took 152.64 seconds and the database occupied about 1.31 GiB including indexes.

| Entity | Rows |
| --- | ---: |
| Users | 5,000 |
| Projects | 2,000 |
| Project memberships | 54,050 |
| Invitations | 50,000 |
| Tasks | 500,000 |
| Task dependencies | 1,000,000 |
| Materials | 20,000 |
| Stock entries | 820,000 |
| Deliveries | 400,000 |
| Delivery lines | 3,000,000 |

The largest project had 3,000 tasks and 50 members. Post-load SQL checks found zero projects without an admin, zero edges violating the seed's topological order, and zero started tasks with an unfinished prerequisite. Project stock balances, outbound reserves and last-delivery timestamps matched the generated history. The dataset had 4,000 active deliveries and 100,000 deliveries sent from projects back to the company store.

## Graph regression

A separate PostgreSQL test fixture had a two-task component next to 179,700 unrelated edges in the same project. After restricting the graph query to reachable nodes, task detail and task edit each allocated about 47 KB in the test process, down from about 35 MB. The query plan used forward and reverse dependency indexes and returned only the relevant edge.

## HTTP reads on the full dataset

A local Go API process queried the Docker PostgreSQL instance over loopback. Each of 28 authenticated cases made four sequential HTTP requests, including JSON response parsing. The table shows representative cases with the first observation and the median of the remaining three requests. The database had already been queried before this pass. These measurements cover local sequential use.

| Screen or query | First observed | Repeat median |
| --- | ---: | ---: |
| Notifications | 5.1 ms | 2.3 ms |
| Unread count | 1.2 ms | 1.1 ms |
| Projects by name | 45.1 ms | 27.9 ms |
| Projects by completion | 65.2 ms | 53.2 ms |
| Late projects by late-task count | 101.8 ms | 86.0 ms |
| Member-scoped projects | 16.0 ms | 15.1 ms |
| Tasks by ID in the 3,000-task project | 15.6 ms | 4.9 ms |
| Tasks by direct dependent count | 11.8 ms | 10.7 ms |
| Task details in the 3,000-task project | 6.4 ms | 3.0 ms |
| Project members | 16.6 ms | 2.5 ms |
| Pending project invitations | 8.1 ms | 2.2 ms |
| Material catalogue | 4.0 ms | 1.2 ms |
| Company stock by status | 15.8 ms | 15.2 ms |
| Project stock by last delivery | 6.0 ms | 3.9 ms |
| Company deliveries | 26.2 ms | 5.6 ms |
| Company deliveries in a sent-date range | 5.9 ms | 4.8 ms |
| Project deliveries | 3.9 ms | 3.3 ms |

Each of the five task sorts traversed the largest project in 30 cursor pages and returned all 3,000 task IDs once. Delivery date queries used the corresponding newest-first indexes for their coarse timestamp bounds, then checked the exact local dates.

An interleaved before-and-after run returned identical project responses and cursors. Grouping late counts across visible projects and disabling JIT inside the list transaction reduced the repeat median for global late ranking from 203.7 ms to 85.2 ms. The first measured request in that run fell from 314.5 ms to 89.1 ms. Completion retained its per-project index-only ranking and its repeat median fell from 65.1 ms to 54.8 ms.
