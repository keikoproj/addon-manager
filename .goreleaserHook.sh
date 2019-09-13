export COMMIT=`git rev-parse HEAD`
export BUILD=`date +%FT%T%z`
export VERSION=`git describe --tags --abbrev=0`