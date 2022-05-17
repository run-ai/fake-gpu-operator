
import requests
from requests.auth import HTTPBasicAuth
import json
import datetime
import os
import re



####  CUSTOM JIRA FIELDS

# master repos: customfield_10080
# tagged repos: customfield_10081
# production repos: customfield_10089


# cluster version: customfield_10087
# backend version: customfield_10088
# CLI version: customfield_10092
# Admin CLI version: customfield_10093
# last prod update date: customfield_10076 
# code status customfield_10074

WORKFLOW_DEBUG=False


# ENV VARs
if WORKFLOW_DEBUG == True:
    print ('debug')
    JIRA_USER=os.environ['JIRA_USER']               # must be in .zshrc
    JIRA_API_TOKEN=os.environ['JIRA_API_TOKEN']     # must be in .zshrc
    GITHUB_REPOSITORY="run-ai/backend"
    RELEASE_VERSION = "1.2.3.4.5"
else:
    JIRA_USER="jira-bot@run.ai"
    GITHUB_REPOSITORY = os.environ['GITHUB_REPOSITORY']
    JIRA_API_TOKEN = os.environ['JIRA_API_TOKEN']
    GITHUB_REF = os.environ['GITHUB_REF']
    RELEASE_VERSION = re.search('refs/tags/(.+)?', GITHUB_REF).group(1)


# CONSTANTS
standalonerepos=["run-ai/runai-cli", "run-ai/runai-admin-cli"]
standlonerepos_customfields=["10092", "10093"]
base_jira_url = "https://runai.atlassian.net/rest/api/3"
clusterrepos = [
    "run-ai/admission-controller",
    "run-ai/agent",
    "run-ai/cluster-sync"
    "run-ai/core-ui",
    "run-ai/elements",
    "run-ai/gpu-monitoring-tools",
    "run-ai/init-ca",
    "run-ai/mig-provisioner",
    "run-ai/nvidia-device-plugin",
    "run-ai/ocp-idp-init",
    "run-ai/project-controller",
    "run-ai/researcher-service", 
    "run-ai/runai-chart",
    "run-ai/runai-container-toolkit-exporter",
    "run-ai/runai-container-toolkit", 
    "run-ai/runai-operator",
    "run-ai/runai-scheduler", 
    "run-ai/runaijob-controller",
    "run-ai/workload-controller",
]
backendrepos = [
    "run-ai/backend", 
    "run-ai/db-mechanic"
]

                   



auth = HTTPBasicAuth(JIRA_USER, JIRA_API_TOKEN)

headers = {
   "Accept": "application/json",
   "Content-Type": "application/json"
}



### JIRA API CALLS

def build_operator_query(repos):
    jql = ""
    for repo in repos:
        if jql != "":
            jql += " OR "
        jql = jql + '"Tagged repos[Labels]" in ("' + repo + '") AND ("Production repos[Labels]" not in ("' + repo + '") or "Production repos[Labels]" is Empty)'
    return jql


def jira_get_jql(fields, jql):
    query = {
    'fields' : fields,
    'jql': jql
    }
    response = requests.request("GET", base_jira_url + "/search", params=query, headers=headers, auth=auth)
    if response.status_code //100 != 2:
        print (response.text)
    else:
        js = json.loads(response.text)
        print(json.dumps(js, sort_keys=True, indent=4, separators=(",", ": ")))
        return js["issues"]


def jira_set_comment (key, comment):
    payload =  {
        "body": {
            "type": "doc",
            "version": 1,
            "content": [
            {
                "type": "paragraph",
                "content": [
                {
                    "text": comment,
                    "type": "text"
                }
                ]
            }
            ]
        }
    }
    response = requests.request("POST", base_jira_url + "/issue/" + key  + "/comment", json=payload, headers=headers, auth=auth)
    if response.status_code //100 != 2:
        print (response.text)
#    print(json.dumps(json.loads(response.text), sort_keys=True, indent=4, separators=(",", ": ")))

    return



def jira_update_fields(key, fields):
    payload = { "update" : fields }
    response = requests.request("PUT", base_jira_url + "/issue/" + key, json=payload, headers=headers, auth=auth)
    if response.status_code //100 != 2:
        print (response.text)
    return response



### PROCESSING

# Tagging of a micro-service that is immediately releasable
def process_standalone_repo_release():
    print("option=independent-microservice")
    now = datetime.datetime.utcnow().isoformat()

    i = standalonerepos.index(GITHUB_REPOSITORY)
    customfield = standlonerepos_customfields[i]

    issues = jira_get_jql("key", '"Master repos[Labels]" in ("' + GITHUB_REPOSITORY  +  '") and ( "Tagged repos[Labels]" is EMPTY or "Tagged repos[Labels]" not in ("' + GITHUB_REPOSITORY  +  '"))')
    for issue in issues:
        print(issue["key"])

        # - set "Last production change date" (customfield_10076)
        # - Update code status to "Production" (customfield_10074)
        # - set "Production backend version" (customfield_10088)
        # - add new repo to Production repos and Tagged repos
        fields = {
            "customfield_" + customfield  : [{"set" :  RELEASE_VERSION }], 
            "customfield_10076" : [{"set" :  now }], 
            "customfield_10074" : [{"set" : "Production" }],
            "customfield_10081" : [{"add" : GITHUB_REPOSITORY }],
            "customfield_10089" : [{"add" : GITHUB_REPOSITORY }]
        }
        jira_update_fields(issue["key"], fields)


        # Add a release comment to Jira
        jira_set_comment(issue["key"], "released-to-production: " + GITHUB_REPOSITORY + ". tag: " + RELEASE_VERSION)
    return



# Tagging of a cluster which effectively moves all dependents to production

def process_cluster_release():
    print("option=cluster")
    now = datetime.datetime.utcnow().isoformat()


    #  Get a subset of tickets that is small enough to inspect due to JQL limitations
    #    issues = jira_get_jql("key,customfield_10089,customfield_10081", '"Tagged repos[Labels]" is not EMPTY and cf[10087] is EMPTY')
    issues = jira_get_jql("key,customfield_10089,customfield_10081", build_operator_query(clusterrepos))
    #        issues = jira_get_jql("key,customfield_10089,customfield_10081", 'KEY=RUN-442')
    for issue in issues:
        print(issue["key"])

        # Get list of cluster repos that are being added to production

        prod_repos = issue["fields"]["customfield_10089"]
        tagged_repos = issue["fields"]["customfield_10081"]
        if prod_repos == tagged_repos:
            continue

        if prod_repos == None:
            repos_diff = tagged_repos
        else:
            repos_diff = list(set(tagged_repos) - set(prod_repos))

        cluster_repos_diff = []
        for repo in repos_diff:
            if repo in clusterrepos:
                cluster_repos_diff.append({"add" : repo })

        if not cluster_repos_diff:
            continue

        # There are tagged micro-services not in prod, must update ticket
        # - Add the new repos to "Prod repos" Jira field 
        # - Update cluster version 
        # - Update last prod update date
        # - Update code status to "Production"

        fields = {
            "customfield_10089": cluster_repos_diff, 
            "customfield_10087":[{"set" : RELEASE_VERSION }],
            "customfield_10076": [{"set" : now }], 
            "customfield_10074" : [{"set" : "Production" }]  
        }
        jira_update_fields(issue["key"], fields)

        # Add a release comment to Jira
        jira_set_comment (issue["key"], "released-cluster-to-production. tag: " + RELEASE_VERSION)
    return



# Tagging of backend which effectively moves all dependents to production

def process_backend_release():
    print("option=backend")
    now = datetime.datetime.utcnow().isoformat()

#  Get a subset of tickets that is small enough to inspect due to JQL limitations
    issues = jira_get_jql("key,customfield_10089,customfield_10081", build_operator_query(backendrepos))
#        issues = jira_get_jql("key,customfield_10089,customfield_10081", 'KEY=RUN-442')
    for issue in issues:
        print(issue["key"])

        # Get list of cluster repos that are being added to production

        prod_repos = issue["fields"]["customfield_10089"]
        tagged_repos = issue["fields"]["customfield_10081"]
        if prod_repos == tagged_repos:
            continue

        if prod_repos == None:
            repos_diff = tagged_repos
        else:
            repos_diff = list(set(tagged_repos) - set(prod_repos))

        backend_repos_diff = []
        for repo in repos_diff:
            if repo in backendrepos:
                backend_repos_diff.append({"add" : repo })

        if not backend_repos_diff:
            continue

        # There are tagged micro-services not in prod, must update ticket
        # - Add the new repos to "Prod repos" Jira field 
        # - Update backend version 
        # - Update last prod update date
        # - Update code status to "Production"

        fields = {
            "customfield_10089": backend_repos_diff, 
            "customfield_10088":[{"set" : RELEASE_VERSION }],
            "customfield_10076": [{"set" : now }], 
            "customfield_10074" : [{"set" : "Production" }]  
        }
        jira_update_fields(issue["key"], fields)

        # Add a release comment to Jira
        jira_set_comment (issue["key"], "released-backend-to-production. tag: " + RELEASE_VERSION)

    return


# Tagging of a micro-service that is not immediately releasable
def process_micro_service_tagging(github_repo):

    print("option=micro-service") 
    issues = jira_get_jql("key", '"Master repos[Labels]" in  ("' + github_repo  +  '") and ( "Tagged repos[Labels]" is EMPTY or "Tagged repos[Labels]" not in ("' + github_repo  +  '"))')

    for issue in issues:
        jira_set_comment (issue["key"], "micro-service-tagged-for-production at: " + github_repo + ". tag: " + RELEASE_VERSION)
        
        jira_update_fields(issue["key"], {"customfield_10081":[{"add": github_repo}]})



def main():
    print(GITHUB_REPOSITORY)
    print(RELEASE_VERSION)

    if GITHUB_REPOSITORY in standalonerepos:
        process_standalone_repo_release()
        return

    process_micro_service_tagging(GITHUB_REPOSITORY)

    if GITHUB_REPOSITORY == "run-ai/runai-chart":
        process_cluster_release()
        return

    if GITHUB_REPOSITORY == "run-ai/backend":
        process_backend_release()
        return

    return

# print(build_operator_query(backendrepos))
# exit(0)

if __name__ == "__main__":
   main()