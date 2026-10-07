#!/bin/sh
# Dev-only. Runs once, when the MySQL data volume is first created. Lets the application
# user create and drop the per-test databases made by internal/db/dbtest (smem_test_<random>);
# it gets nothing on any other schema.
set -eu
mysql -uroot -p"$MYSQL_ROOT_PASSWORD" <<SQL
GRANT ALL PRIVILEGES ON \`smem\_test\_%\`.* TO '$MYSQL_USER'@'%';
SQL
