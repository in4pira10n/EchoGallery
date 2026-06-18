# User Management

EchoGallery now provides a dedicated root console for user management and visitor library authorization.

## Root Console

Run beside the EchoGallery binary:

```bash
./EchoGallery root
./EchoGallery root 9090
```

This starts a special web console without normal account login.
Use it to:

- create `admin` and `visitor` users
- rename users
- change passwords
- delete users
- grant visitor library access
- choose each user's default library

New visitors cannot log in until root grants at least one library.

## Commands

Run commands beside the EchoGallery binary:

```bash
./EchoGallery users
./EchoGallery adduser
./EchoGallery moduser <username>
./EchoGallery deluser <username>
./EchoGallery setuserrole <username>
./EchoGallery setuserlibs <username>
```

If you build from source and run the shell script output, replace `./EchoGallery` with the actual binary path.

## What Each Command Does

- `users`
  - Lists every user, role, default library, and allowed library IDs.
- `adduser`
  - Interactive wizard.
  - Creates either an `admin` or `visitor`.
- `moduser <username>`
  - Renames a user and/or changes the password.
- `deluser <username>`
  - Deletes the user and that user's profile directory.
  - EchoGallery keeps at least one user.
- `setuserrole <username>`
  - Switches between `admin` and `visitor`.
- `setuserlibs <username>`
  - Sets the visitor's allowed libraries and default library.
  - For admins, all libraries are always allowed; this command only adjusts the default library.

## Current Rules

- Admins are peers.
- Admins can access all libraries unless a library is currently occupied by another admin instance.
- Visitors can only access libraries explicitly approved for them.
- Visitor authorization is stored in `config.json`.
- Personal preferences remain in each user's `profile.json`.

## Recommended Workflow

1. Run `./EchoGallery users` to copy the current library IDs.
2. Run `./EchoGallery adduser` and choose `visitor` when needed.
3. Run `./EchoGallery setuserlibs <username>` to grant libraries.
4. Ask the visitor to sign in again if their access changed while they were online.

## Notes

- Upgrading from older builds will migrate legacy `profile.json` library lists into the shared global library registry at startup.
- If you also enable `清理文件` in batch workflow, EchoGallery will rewrite migrated legacy config/profile data and cleanup stale lock rows before continuing.
