#!/usr/bin/env bash

#
# Copyright 2023 Stacklok, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

# Wait for Keycloak to start and authenticate with admin credentials
while ! /opt/keycloak/bin/kcadm.sh config credentials --server http://keycloak:8080 --realm master --user "$KEYCLOAK_ADMIN" --password "$KEYCLOAK_ADMIN_PASSWORD"; do
  sleep 1
done

# Create realm stacklok
/opt/keycloak/bin/kcadm.sh create realms -s realm=stacklok -s loginTheme=keycloak -s enabled=true

# Create client mediator-cli
/opt/keycloak/bin/kcadm.sh create clients -r stacklok -s clientId=mediator-cli -s 'redirectUris=["http://localhost/*"]' -s publicClient=true -s enabled=true

# Create client mediator-ui
/opt/keycloak/bin/kcadm.sh create clients -r stacklok -s clientId=mediator-ui -s 'redirectUris=["http://localhost/*"]' -s publicClient=true -s enabled=true

# Add account deletion capability to stacklok realm (see https://www.keycloak.org/docs/latest/server_admin/#authentication-operations)
/opt/keycloak/bin/kcadm.sh update "/authentication/required-actions/delete_account" -r stacklok -b '{ "alias" : "delete_account", "name" : "Delete Account", "providerId" : "delete_account", "enabled" : true, "defaultAction" : false, "priority" : 60, "config" : { }}'

# Give all users permission to delete their own account
/opt/keycloak/bin/kcadm.sh add-roles -r stacklok --rname default-roles-stacklok --rolename delete-account --cclientid account
