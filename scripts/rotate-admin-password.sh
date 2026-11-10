#!/usr/bin/env bash
#
# Sets a new password for the administrator (default: BOOTSTRAP_ADMIN_USER) and prints it. The user must
# change it at the next sign-in, and every existing session of that user is signed out.
#
#   make admin-password                         random password for the first admin
#   make admin-password U=ali                   another user
#   make admin-password U=ali PASSWORD='...'    choose your own (>= 12 chars)
source "$(dirname "$0")/lib.sh"
[[ -f .env ]] || die ".env not found - run: make env"
USERN="${U:-$(env_get BOOTSTRAP_ADMIN_USER)}"; USERN="${USERN:-admin}"
new="${PASSWORD:-$(rand_secret 18)}"
[[ ${#new} -ge 12 ]] || die "The password must be at least 12 characters."
bash scripts/admin.sh reset-password -username "$USERN" -password "$new"
[[ "$USERN" == "$(env_get BOOTSTRAP_ADMIN_USER)" ]] && set_env_value .env BOOTSTRAP_ADMIN_PASSWORD "$new"
echo ""
echo "Login:  ${USERN} / ${new}"
echo "(the user will be asked to choose their own password at first sign-in)"
