#!/bin/sh
# SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later
#
# package-linux.sh turns the staged Linux bundle into Debian, Arch Linux
# (pacman) and RPM packages.
#
# Run scripts/build-linux.sh first. The packages are written to
# dist/packages/.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stage=${LINUX_STAGE:-$root/dist/linux}
out=${PACKAGE_DIR:-$root/dist/packages}
version=${VERSION:-0.1.0}
target_arch=${TARGET_ARCH:-amd64}
pkgrel=${PKGREL:-1}
url=https://github.com/Ne0W0r1d/zephyr-center
maintainer="Ne0W0r1d <Ne0W0r1d@users.noreply.github.com>"

fail() {
  echo "package-linux: $*" >&2
  exit 1
}

case "$version" in
  [0-9]*) ;;
  *) fail "VERSION must start with a digit: $version" ;;
esac
case "$version" in
  ''|*[!A-Za-z0-9.+~_-]*) fail "VERSION contains unsupported characters: $version" ;;
  *-) fail "VERSION must not end with a hyphen: $version" ;;
esac

deb_upstream=$version
deb_revision=$pkgrel
case "$version" in
  *-[0-9]*)
    suffix=${version##*-}
    case "$suffix" in
      *[!0-9]*) ;;
      *)
        deb_upstream=${version%-*}
        deb_revision=$suffix
        ;;
    esac
    ;;
esac
deb_upstream=$(printf '%s' "$deb_upstream" |
  tr '_' '.' | sed 's/-/~/g')
deb_pkg_version="$deb_upstream-$deb_revision"

case "$target_arch" in
  amd64|x86_64)
    deb_arch=amd64
    arch_arch=x86_64
    rpm_arch=x86_64
    ;;
  arm64|aarch64)
    deb_arch=arm64
    arch_arch=aarch64
    rpm_arch=aarch64
    ;;
  *) fail "unsupported TARGET_ARCH: $target_arch" ;;
esac

for tool in dpkg-deb rpmbuild zstd; do
  command -v "$tool" >/dev/null 2>&1 ||
    fail "missing $tool (install dpkg-dev, rpm and zstd)"
done
[ -x "$stage/zephyr-center" ] ||
  fail "no Linux build in $stage; run scripts/build-linux.sh with VERSION set"

work=$(mktemp -d "${TMPDIR:-/tmp}/zephyr-center-package.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM

pkgroot=$work/root
install -Dm755 "$stage/zephyr-center" "$pkgroot/usr/bin/zephyr-center"
cp -a "$stage/share" "$pkgroot/usr/share/"
install -Dm644 "$stage/README.md" "$pkgroot/usr/share/doc/zephyr-center/README.md"
install -Dm644 "$stage/NOTICE" "$pkgroot/usr/share/doc/zephyr-center/NOTICE"
install -Dm644 "$stage/LICENSE" "$pkgroot/usr/share/doc/zephyr-center/copyright"
install -Dm644 "$stage/LICENSE" "$pkgroot/usr/share/licenses/zephyr-center/LICENSE"
install -Dm644 "$stage/NOTICE" "$pkgroot/usr/share/licenses/zephyr-center/NOTICE"

installed_kb=$(du -sk "$pkgroot" | awk '{print $1}')
installed_bytes=$(du -sb "$pkgroot" | awk '{print $1}')
builddate=$(date +%s)

mkdir -p "$out"
rm -f "$out"/zephyr-center*

debroot=$work/debian
cp -a "$pkgroot" "$debroot"
mkdir -p "$debroot/DEBIAN"
cat >"$debroot/DEBIAN/control" <<EOF
Package: zephyr-center
Version: $deb_pkg_version
Architecture: $deb_arch
Maintainer: $maintainer
Installed-Size: $installed_kb
Depends: libc6, libgtk-3-0 | libgtk-3-0t64, libwebkit2gtk-4.1-0 | libwebkit2gtk-4.0-37
Section: utils
Priority: optional
Homepage: $url
Description: Control center for ASUS laptops
 Zephyr Center controls performance profiles, fans, Aura lighting and
 graphics switching on ASUS laptops. On Linux it uses asusd over D-Bus
 with a kernel sysfs fallback.
EOF
deb_file="$out/zephyr-center_${deb_pkg_version}_${deb_arch}.deb"
dpkg-deb --root-owner-group --build "$debroot" "$deb_file" >/dev/null
actual_deb_version=$(dpkg-deb --field "$deb_file" Version)
[ "$actual_deb_version" = "$deb_pkg_version" ] ||
  fail "Debian package version is $actual_deb_version, expected $deb_pkg_version"

arch_pkgver=$version
arch_pkgrel=$pkgrel
case "$version" in
  *-[0-9]*)
    suffix=${version##*-}
    case "$suffix" in
      *[!0-9]*) ;;
      *)
        arch_pkgver=${version%-*}
        arch_pkgrel=$suffix
        ;;
    esac
    ;;
esac
arch_pkgver=$(printf '%s' "$arch_pkgver" | sed 's/[^A-Za-z0-9._+]/_/g')
case "$arch_pkgrel" in
  ''|*[!0-9]*) fail "PKGREL must be a positive integer, got: $arch_pkgrel" ;;
esac
arch_fullver="$arch_pkgver-$arch_pkgrel"

archroot=$work/arch
cp -a "$pkgroot" "$archroot"
cat >"$archroot/.PKGINFO" <<EOF
pkgname = zephyr-center
pkgbase = zephyr-center
pkgver = $arch_fullver
pkgdesc = Control center for ASUS laptops
url = $url
builddate = $builddate
packager = $maintainer
size = $installed_bytes
arch = $arch_arch
license = MPL-2.0
license = GPL-3.0-or-later
depend = glibc
depend = gtk3
depend = webkit2gtk-4.1
optdepend = asusctl: fan curves and Aura lighting over D-Bus
EOF

arch_file="$out/zephyr-center-${arch_fullver}-${arch_arch}.pkg.tar.zst"
if command -v bsdtar >/dev/null 2>&1; then
  (
    cd "$archroot"
    bsdtar -cf - --format=mtree \
      --options='!all,use-set,type,uid,gid,mode,time,size,md5,sha256,link' \
      --uid 0 --gid 0 .PKGINFO usr | gzip -9n >.MTREE
    bsdtar -cf - --uid 0 --gid 0 --uname root --gname root \
      --no-xattrs --no-acls \
      .PKGINFO .MTREE usr | zstd -19 -T0 -q -o "$arch_file"
  )
else
  echo "package-linux: bsdtar not found, building without .MTREE" >&2
  (
    cd "$archroot"
    tar --owner=0 --group=0 --numeric-owner --no-xattrs --no-acls \
      -cf - .PKGINFO usr |
      zstd -19 -T0 -q -o "$arch_file"
  )
fi

zstd -dc "$arch_file" | tar -xOf - .PKGINFO >"$work/PKGINFO"
if ! grep -qx "pkgver = $arch_fullver" "$work/PKGINFO"; then
  fail "Arch package does not declare pkgver = $arch_fullver"
fi
case "$arch_fullver" in
  *-*) ;;
  *) fail "Arch pkgver must include a pkgrel: $arch_fullver" ;;
esac

rpm_version=$version
rpm_release=$pkgrel
case "$version" in
  *-*)
    rpm_version=${version%%-*}
    rpm_release=${version#*-}
    ;;
esac
rpm_version=$(printf '%s' "$rpm_version" | sed 's/[^A-Za-z0-9._+~^]/_/g')
rpm_release=$(printf '%s' "$rpm_release" | sed 's/[^A-Za-z0-9._+~^]/_/g')
case "$rpm_version" in
  ''|.*|*.) fail "RPM Version is empty or starts or ends with a dot: $rpm_version" ;;
esac
case "$rpm_release" in
  ''|.*|*.) fail "RPM Release is empty or starts or ends with a dot: $rpm_release" ;;
esac

spec=$work/zephyr-center.spec
cat >"$spec" <<EOF
Name: zephyr-center
Version: $rpm_version
Release: $rpm_release
Summary: Control center for ASUS laptops
License: MPL-2.0 OR GPL-3.0-or-later
URL: $url
BuildArch: $rpm_arch
Packager: $maintainer
AutoReqProv: no
Requires: glibc
Requires: gtk3
Requires: webkit2gtk4.1

%description
Zephyr Center controls performance profiles, fans, Aura lighting and
graphics switching on ASUS laptops. On Linux it uses asusd over D-Bus
with a kernel sysfs fallback.

%install
mkdir -p "%{buildroot}"
cp -a "%{pkgroot}/." "%{buildroot}/"

%files
%defattr(-,root,root,-)
%attr(0755,root,root) /usr/bin/zephyr-center
/usr/share/applications/io.github.ne0w0r1d.ZephyrCenter.desktop
/usr/share/icons/hicolor
%doc /usr/share/doc/zephyr-center/README.md
%doc /usr/share/doc/zephyr-center/NOTICE
%doc /usr/share/doc/zephyr-center/copyright
%license /usr/share/licenses/zephyr-center/LICENSE
%license /usr/share/licenses/zephyr-center/NOTICE

%changelog
* $(date '+%a %b %d %Y') $maintainer - $rpm_version-$rpm_release
- Automated build.
EOF

topdir=$work/rpmbuild
mkdir -p "$topdir/BUILD" "$topdir/BUILDROOT" "$topdir/RPMS" \
  "$topdir/SOURCES" "$topdir/SPECS" "$topdir/SRPMS"
rpmbuild --quiet -bb --define "_topdir $topdir" --define "pkgroot $pkgroot" \
  --define "_build_id_links none" "$spec" >/dev/null
built_rpm=$(find "$topdir/RPMS" -type f -name '*.rpm' -print -quit)
[ -n "$built_rpm" ] || fail "rpmbuild produced no package"
rpm_file="$out/zephyr-center-${rpm_version}-${rpm_release}.${rpm_arch}.rpm"
mv "$built_rpm" "$rpm_file"

echo "wrote packages to $out"
ls -1 "$out"
