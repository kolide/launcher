#!/bin/bash

# This is a helper shell script to add things to projects. It's based
# on
# https://docs.github.com/en/issues/trying-out-the-new-projects-experience/automating-projects
#
# It's really designed around the workflow use case. Thus a bit
# awkward for command line usage. But, you should be able to set
# appropriate variables and call it.
#
# Most of the vars are straight forward, but the CONTENT_ID is the
# global id for the PR or Issue (either works). finding it is a
# huuuuge hassle. Luckily it's in the workflow directly, but from
# outside it, it's a lot harder. Simplest way I've found is:
#
#     gh -R kolide/launcher issue list -l 'global check' --json title,id

for var in GITHUB_TOKEN ORGANIZATION PROJECT_NUMBER CONTENT_ID TMPDIR; do
    if [ -z "${!var}" ]; then
       echo "$var is unset"
       exit 1
    fi
done

set -e -o pipefail

DIR=$(mktemp -d "$TMPDIR/github-add-to-project-XXXXX")
cd $DIR

##
## Fetch Project Metadata
##

gh api graphql --header 'GraphQL-Features: projects_next_graphql' -f query='
            query($org: String!, $number: Int!) {
              organization(login: $org){
                projectNext(number: $number) {
                  id
                  fields(first:20) {
                                   nodes {
                      id
                      name
                      settings
                    }
                  }
                }
              }
            }' -f org=$ORGANIZATION -F number=$PROJECT_NUMBER > project_data.json


# quoting is really weird and inconsistent. Some need the quotes, some do not.
PROJECT_ID=$(jq  '.data.organization.projectNext.id' project_data.json)
DATE_FIELD_ID=$(jq '.data.organization.projectNext.fields.nodes[] | select(.name== "Date Added To Board") | .id' project_data.json)
STATUS_FIELD_ID=$(jq '.data.organization.projectNext.fields.nodes[] | select(.name== "Status") | .id' project_data.json)
TRIAGE_OPTION_ID=$(jq -rc '.data.organization.projectNext.fields.nodes[] | select(.name== "Status") |.settings | fromjson.options[] | select(.name=="Triage") |.id' project_data.json)

##
## Add target to project
##

# This returns the card's global id, which we need for any subsequent editing

CARD_ID=$(gh api graphql --header 'GraphQL-Features: projects_next_graphql' -f query='
            mutation($project:ID!, $target:ID!) {
              addProjectNextItem(input: {projectId: $project, contentId: $target}) {
                projectNextItem {
                  id
                }
              }
            }' -f project=$PROJECT_ID -f target=$CONTENT_ID --jq '.data.addProjectNextItem.projectNextItem.id')

##
## Set Fields
##

# Unlike the github examples, we don't want set any fields. I can't tell how to set-if-unset, and I don't want to overwrite.

#DATE=$(date +"%Y-%m-%d")
#gh api graphql --header 'GraphQL-Features: projects_next_graphql' -f query='
#            mutation (
#              $project: ID!
#              $item: ID!
#              $status_field: ID!
#              $status_value: String!
#              $date_field: ID!
#              $date_value: String!
#            ) {
#              set_status: updateProjectNextItemField(input: {
#                projectId: $project
#                itemId: $item
#                fieldId: $status_field
#                value: $status_value
#              }) {
#                projectNextItem {
#                  id
#                  }
#              }
#              set_date_posted: updateProjectNextItemField(input: {
#                projectId: $project
#                itemId: $item
#                fieldId: $date_field
#                value: $date_value
#              }) {
#                projectNextItem {
#                  id
#                }
#              }
#            }' -f project=$PROJECT_ID -f item=$CARD_ID -f status_field=$STATUS_FIELD_ID -f status_value=$TRIAGE_OPTION_ID -f date_field=$DATE_FIELD_ID -f date_value=$DATE --silent

rm -rf "$DIR"
