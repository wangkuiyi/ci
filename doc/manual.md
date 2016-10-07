# User's Manual

## Setup MySQL

CI uses MySQL as its backend storage.  From the command line, we can specify

1. MySQL username
1. MySQL password
1. MySQL database

CI needs a table called `ci` in the specified database with the
following schema:

- Column 1: `id`, STRING, the Git commit ID (SHA).
- Column 2: `status`, STRING, can take value from `pending`, `success`, and `failed`.
- Column 3: `detail`, LONGTEXT, the stdout and stderr from running the `.ci.bash` script.

We can use Homebrew to setup MySQL on Mac OS X:

```
brew install mysql
```

Then we need to initialize MySQL -- creating a root account in a
specified data directory:

```
mysqld --initialize-insecure --datadir=/Users/yiwang/work/mysql_data
```

For development purpose, the root user can have empty password, so we
use `--initialize-insecure`.  For deployment purpose, we might need
`--initialize` instead.

Then we can start MySQL daemon:

```
mysql.server restart
```

It is OK to run `mysqld` directly, but that will cost you a terminal
window.

How we can use `mysql` to control MySQL:

```
mysql -u root
```

If we used `mysqld --initialize` to create the root user and gave it a
password, we need to let `mysql` prompt us for the password:

```
mysql -u root -p
```

Then we can create the database under the `mysql` prompt:

```
$ CREATE DATABASE ci;
$ QUIT;
```

and the table `ci` in database `ci`:

```
mysql -u root ci < initialize.mysql
mysql -u root ci < initialize_test.mysql
```

## Setup `ngrok`

We need ngrok, so that we can host CI on home/office computer that
don't have public&static Internet IPs, while allowing it to have a
publicly accessible domain&URL.

We can use Homebrew to install ngrok on Mac OS X:

```
brew cask install ngrok
```

Run ngrok on our computer and expose local :8080 port to ngrok.com, so
that our CI process, which listens on :8080, could have a publicly
accessible URL, which can be registred to github.com as a Webhook:

```
ngrok http 8080
```

This will bring up a fullscreen UI like the following:

<img src="ngrok.png" width=500 />

Please be aware of the URLs shown in the figure -- once we run CI
locally and make it listening on 8080, we will be able to access CI
through any of those URLs.  Also, once after we run CI on port 8080,
we can access locally running ngrok through
http://localhost:4040/inspect/http.
