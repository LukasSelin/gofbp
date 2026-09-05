# The oracle, pinned. Builds an image that can run gen_cffdrs_reference.R, so
# regenerating cffdrs.json needs Docker and nothing else -- no R on the host, no
# hunting for a GDAL that matches.
#
# Both versions are recorded INTO the fixture (oracle/cffdrs_version/r_version at
# its top level) and logged by every Go TestCFFDRS*, so a fixture regenerated
# against different versions than the last one announces itself in the diff rather
# than silently shifting a few thousand reference numbers. Change them here, not
# at the call site; regenerate; read the diff.
#
# Built and run by regen-cffdrs.sh. See that script for usage.
ARG R_VERSION=4.6.1
FROM rocker/r-ver:${R_VERSION}

# cffdrs Imports sf and terra, which link against GDAL/GEOS/PROJ. We use none of
# that -- fbp() is arithmetic over a data.frame -- but R resolves every Import at
# load time, so the libraries have to be here for library(cffdrs) to return.
#
# The -dev packages rather than the versioned runtime ones (libgdal36t64 and
# friends): they pull the runtime in as a dependency and their names do not
# change under us when the base image's Ubuntu release moves.
RUN apt-get update && apt-get install -y --no-install-recommends \
        libgdal-dev \
        libgeos-dev \
        libproj-dev \
        libudunits2-dev \
        libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

# rocker/r-ver pins a dated CRAN snapshot, so install.packages() is already
# reproducible -- but it resolves to whatever cffdrs was current at that date.
# install_version pins the number we actually assert against.
ARG CFFDRS_VERSION=1.9.2
RUN R -q -e "install.packages('remotes')" \
    && R -q -e "remotes::install_version('cffdrs', version='${CFFDRS_VERSION}', upgrade='never')" \
    && R -q -e "stopifnot(packageVersion('cffdrs') == '${CFFDRS_VERSION}'); suppressMessages(library(cffdrs)); cat('cffdrs', as.character(packageVersion('cffdrs')), 'loads OK\n')"

# The script writes to testdata/cffdrs.json relative to the module root, so it
# must run from there. regen-cffdrs.sh mounts the module root here.
WORKDIR /repo
ENTRYPOINT ["Rscript", "testdata/gen_cffdrs_reference.R"]
