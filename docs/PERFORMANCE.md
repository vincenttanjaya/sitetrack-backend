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
