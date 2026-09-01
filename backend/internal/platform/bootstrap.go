package platform

// BootstrapAdminEmail is read from the environment by Config.
//
// The chicken-and-egg problem it solves: inviting a user requires an admin, and
// a fresh deployment has none. The old answer was a credential literal in the
// login form (`admin@admin.com / admin`), which is a permanent backdoor
// answering a one-time question.
//
// The rules that keep this from becoming the same thing:
//
//  1. **It runs only when there is no active admin.** On every later boot it is
//     a no-op, so leaving the variable set in a deployment does not re-open
//     anything.
//  2. **It sets no password.** It creates an invitation, exactly as an admin
//     inviting a colleague would, and the one-time token is printed to the
//     startup log for whoever is doing the deployment. Nothing is guessable and
//     nothing is shared.
//  3. **The token expires.** An unused bootstrap invite dies on the same clock
//     as any other (InviteTTL), so a forgotten deployment does not leave a live
//     way in indefinitely.
//
// The token appearing in a log is a real, accepted trade: the log is already
// trusted with the DSN's existence and the deployment operator is reading it at
// that moment. `WI-79` replaces this with email delivery.
const BootstrapEnvVar = "BOOTSTRAP_ADMIN_EMAIL"
