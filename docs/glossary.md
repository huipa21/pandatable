# Glossary

| Term | Meaning |
|---|---|
| **Workspace** | Top-level container, owned by one or more users. Holds books. |
| **Book** | A group of related tables inside a workspace (= one PostgreSQL schema). Relations only within a book. |
| **Table** | A user-defined table (= one physical PostgreSQL table). |
| **Field** | A column of a table with a UI type. |
| **View** | A saved way to look at a table (grid, form, kanban, …) with its own filter/sort/group/highlight. |
| **Shared view** | View visible to everyone with access to the table. |
| **Personal view** | View visible only to its creator. |
| **Locked view** | Shared view whose config only owners can change. |
| **App role** | `admin` or `user`; only controls Admin Panel access. |
| **Grant** | Permission (`read`, `write`, `owner`) given to a user/group on a workspace/book/table. |
| **Effective permission** | Highest applicable grant along the path for the user and their groups. |
| **Orphaned workspace** | Workspace without any active direct user owner. |
| **Link field** | Field representing a relation to another table (1:1, 1:N, M:N). |
| **Lookup** | Field showing values of a field from linked rows. |
| **Rollup** | Field aggregating values from linked rows. |
| **Display field** | Primary field used to represent a row (in links, cards, titles). |
| **Highlight** | Conditional formatting rule coloring rows or cells. |
| **PandaTable package** | XLSX export with hidden `_pandatable` manifest sheet enabling exact re-import. |
| **Trash** | Soft-deleted structural objects, restorable for 30 days. |
