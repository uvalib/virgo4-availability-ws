#
# build the target application
#
FROM golang:1.16.0-alpine3.13 as builder
RUN apk update && apk upgrade && apk add make

WORKDIR /build
COPY go.mod go.sum Makefile ./
COPY cmd ./cmd
COPY data ./data
ARG GIT_COMMIT
RUN make linux GIT_COMMIT="$GIT_COMMIT"

#
# build the target container
#
FROM alpine:3.13

# update the packages
RUN apk update && apk upgrade && apk add bash tzdata ca-certificates && rm -rf /var/cache/apk/*

# Create the run user and group
RUN addgroup webservice && adduser webservice -G webservice -D

# set the timezone appropriatly
ENV TZ=UTC
RUN cp /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# Specify home
ENV APP_HOME /availability-ws
WORKDIR $APP_HOME

# Create necessary directories
RUN mkdir -p $APP_HOME $APP_HOME/bin $APP_HOME/scripts $APP_HOME/bin/data
RUN chown -R webservice $APP_HOME && chgrp -R webservice $APP_HOME

# port and run command
EXPOSE 8080
CMD scripts/entry.sh

# Move in necessary assets
COPY package/data/container_bash_profile /home/webservice/.profile
COPY package/scripts/entry.sh $APP_HOME/scripts/entry.sh
COPY data $APP_HOME/bin/data
COPY --from=builder /build/bin/v4availability.linux $APP_HOME/bin/v4availability
COPY data/* $APP_HOME/bin/data/

# Ensure permissions are correct
RUN chown webservice:webservice /home/webservice/.profile $APP_HOME/scripts/entry.sh $APP_HOME/bin/v4availability $APP_HOME/bin/data && chmod 755 /home/webservice/.profile $APP_HOME/scripts/entry.sh $APP_HOME/bin/v4availability $APP_HOME/bin/data

# Add the build tag
ARG BUILD_TAG
RUN test -n "$BUILD_TAG" && touch $APP_HOME/buildtag.build-$BUILD_TAG || touch $APP_HOME/buildtag.build-0

# Specify the user
USER webservice

#
# end of file
#
