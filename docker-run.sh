#!/usr/bin/env bash
docker run -p 9292:9292 -v $(pwd)/downloads:/download -e MR_DOWNLOAD_DIR=/download vndangkhoa/kv-download:latest
