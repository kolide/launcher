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

Note that because we currently package our migration files with launcher, it is possible for us to run a migration and then downgrade to a version of launcher which cannot find the corresponding migration file.
To avoid this issue, we ignore missing migration file errors when migrating up on startup. This is enough for now, because all migrations can be run multiple times without error (see best practices note above). If these migrations become more complex, (e.g. table altercations that will break prior launcher versions, instead of completely new tables), we will need to explore another path forward here (likely hosting migrations with rollbacks outside of launcher, so that older versions can rollback newer migrations).
