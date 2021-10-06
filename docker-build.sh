#!/bin/sh
VERSION="keycloak-gosns:latest"
echo "$(oc whoami -t)" | docker login -u $USER --password-stdin https://openshift-image-registry.apps.foresight.aareon.com && \
docker build . -t $VERSION -t openshift-image-registry.apps.foresight.aareon.com/rh-sso/$VERSION && \
docker push openshift-image-registry.apps.foresight.aareon.com/rh-sso/$VERSION