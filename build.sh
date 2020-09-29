#!/bin/bash
home=$(dirname $0)
cd $home

lTag=$(git tag --sort=-creatordate | head -1)
debPkg="$home/frontail_${lTag}_amd64.deb"
if [ -e "$debPkg" ]; then
    echo "You must save your work under a git-tag."
    exit 1
fi
if grep "$lTag" build/debian/changelog; then
    echo "changelog for version $lTag already done"
else
    echo -e "frontail ($lTag) UNRELEASED; urgency=medium\n" > build/debian/changelog.new
    firstline=1
    while IFS= read -r line; do
        if [ -n "$firstline" ]; then
            echo "  * $line" >> build/debian/changelog.new
            firstline=""
        else
            echo "  $line" >> build/debian/changelog.new
        fi
    done <<< $(changelogfromtags --tag $lTag)
    tail -n +2 build/debian/changelog >> build/debian/changelog.new
    mv -f build/debian/changelog build/debian/changelog.old
    mv build/debian/changelog.new build/debian/changelog
fi
find . -maxdepth 1 -type f -exec rm -f build/{} \;

tar czvf frontail_${lTag}.orig.tar.gz Dockerfile frontail.1 frontail.systemd main.go README.md LICENSE Makefile

cd build
tar czf ../frontail_${lTag}.orig.tar.gz
debuild -us -uc

debsign

cd -
dpkg -c $debPkg