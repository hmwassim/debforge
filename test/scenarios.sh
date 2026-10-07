#!/bin/sh
# Executed inside debian:trixie as root.
set -eu
pass() { echo "PASS: $*"; }
fail() { echo "FAIL: $*"; exit 1; }
installed() { dpkg-query -W -f '${db:Status-Status}' "$1" 2>/dev/null | grep -qx installed; }
export SUDO_USER=tester

apt-get update -qq >/dev/null
useradd -m tester
apt-get install -y -qq procps >/dev/null
apt-get install -y -qq curl nano >/dev/null   # pre-existing packages owned by the user
debforge version

# 1. install with dependency; files, ownership, hook
debforge -y install itest-app
installed jq && installed tree || fail "apt packages not installed"
[ "$(cat /etc/itest/base.conf)" = "base=1" ] || fail "system file"
[ "$(stat -c %U /home/tester/.config/itest/user.conf)" = tester ] || fail "user file owner"
[ "$(stat -c %U /home/tester/.config/itest)" = tester ] || fail "user dir owner"
[ -e /tmp/itest-base-hook ] || fail "post_install hook"
pass "install with dependency"

# 2. idempotent
out=$(debforge -y install itest-app 2>&1)
echo "$out" | grep -q "already installed" || fail "second install not a no-op: $out"
pass "idempotent install"

# 3. user edit survives removal; dependents and orphans removed; pre-existing kept
echo "mine" > /etc/itest/base.conf
debforge -y remove itest-base
installed jq && fail "jq should be removed"
installed tree && fail "tree should be removed"
installed curl || fail "pre-existing curl must stay installed"
[ "$(cat /etc/itest/base.conf)" = mine ] || fail "modified file was deleted"
[ ! -e /etc/itest/app.conf ] || fail "unmodified file not removed"
debforge list --installed --names | grep -q itest && fail "state not empty"
pass "remove with dependents, ref-counting and modified-file protection"

# 4. foreign file is backed up and restored
rm -f /etc/itest/base.conf
echo "distro" > /etc/itest/base.conf
debforge -y install itest-base
[ "$(cat /etc/itest/base.conf.debforge-orig)" = distro ] || fail "no backup of foreign file"
debforge -y remove itest-base
[ "$(cat /etc/itest/base.conf)" = distro ] || fail "foreign file not restored"
pass "foreign file backup/restore"

# 5. conflicts removed in the same transaction
debforge -y install itest-conflict
installed vim-tiny || fail "vim-tiny missing"
installed nano && fail "nano should be removed"
debforge -y remove itest-conflict
pass "conflicts"

# 6. corrupt state is moved aside, not fatal
mkdir -p /var/lib/debforge && echo '{oops' > /var/lib/debforge/state.json
debforge -y install itest-base >/tmp/corrupt.log 2>&1
grep -q "moved to" /tmp/corrupt.log || fail "corrupt state not reported"
ls /var/lib/debforge/state.json.corrupt-* >/dev/null || fail "corrupt state not kept"
debforge -y remove itest-base
pass "corrupt state recovery"

# 7. packages removed behind debforge's back are detected
debforge -y install itest-base
apt-get remove -y -qq jq >/dev/null
debforge -n sync >/tmp/sync.log 2>&1
grep -q itest-base /tmp/sync.log || fail "drift not detected"
debforge -y update itest-base >/tmp/update.log 2>&1
grep -qi reinstall /tmp/update.log || fail "update should reinstall missing package"
installed jq || fail "jq not reinstalled"
debforge -y remove itest-base
pass "drift detection and repair"

# 8. Ctrl-C during apt: apt finishes, dpkg is consistent, package is incomplete
debforge -y install itest-big > /tmp/big.log 2>&1 &
pid=$!
i=0
until pgrep -x apt-get >/dev/null || [ $i -ge 300 ]; do sleep 0.1; i=$((i+1)); done
kill -INT $pid
set +e; wait $pid; code=$?; set -e
[ "$code" -eq 130 ] || { cat /tmp/big.log; fail "exit code $code, want 130"; }
[ -z "$(dpkg --audit)" ] || fail "dpkg left inconsistent"
installed gcc || fail "apt transaction was cut off"
debforge info itest-big | grep -q incomplete || fail "not marked incomplete"
debforge -y update itest-big >/dev/null
debforge info itest-big | grep -q "incomplete" && fail "update did not complete it"
pass "Ctrl-C during apt"

# 9. setup and doctor run (dry) without systemd
debforge -n setup >/tmp/setup.log 2>&1 || { cat /tmp/setup.log; fail "setup --dry-run"; }
grep -q "Setup will apply" /tmp/setup.log || fail "setup plan not shown"
set +e; debforge doctor >/dev/null 2>&1; code=$?; set -e
[ "$code" -eq 1 ] || fail "doctor exit $code, want 1"
pass "setup plan and doctor"

# 10. (slow, opt-in) a debconf EULA must not hang: preseeded + noninteractive
if [ "${ITEST_SLOW:-}" = 1 ]; then
    sed -i 's/^Components: main$/Components: main contrib/' /etc/apt/sources.list.d/debian.sources
    apt-get update -qq >/dev/null
    timeout 900 debforge -y install itest-eula </dev/null >/tmp/eula.log 2>&1 || { tail -20 /tmp/eula.log; fail "EULA install"; }
    installed ttf-mscorefonts-installer || fail "mscorefonts not installed"
    pass "debconf EULA does not hang"
fi

echo "ALL SCENARIOS PASSED"
