# Creating a migration

Inside this directory (ee/agent/storage/sqlite), run the command to create a migration as follows:

```
migrate create -ext sqlite -dir migrations -seq <migration name>
```

The migration name is only used to name the file.

This command will create two new files in the migrations/ directory, an up and a down migration.
Add your SQL to these files. Adhere to best practices, including:

1. All migrations should be reversible
1. All migrations should be able to run multiple times without error (e.g. use `CREATE TABLE IF NOT EXISTS` instead of `CREATE TABLE`)

The database will automatically apply new migrations on startup.
