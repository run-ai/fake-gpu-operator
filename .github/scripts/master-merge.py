
import requests
from requests.auth import HTTPBasicAuth
import json
import os

####  CUSTOM JIRA FIELDS
# master repos: customfield_10080


WORKFLOW_DEBUG=False

# ENV VARs
if WORKFLOW_DEBUG == True:
    print ('debug')
    JIRA_USER=os.environ['JIRA_USER']               # must be in .zshrc
    JIRA_API_TOKEN=os.environ['JIRA_API_TOKEN']     # must be in .zshrc
    GITHUB_REPOSITORY="run-ai/backend"
else:
    JIRA_USER="jira-bot@run.ai"
    JIRA_API_TOKEN = os.environ['JIRA_API_TOKEN']
    GITHUB_REPOSITORY = os.environ['GITHUB_REPOSITORY']


# CONSTANTS
base_jira_url = "https://runai.atlassian.net/rest/api/3"



auth = HTTPBasicAuth(JIRA_USER, JIRA_API_TOKEN)

headers = {
   "Accept": "application/json",
   "Content-Type": "application/json"
}



#JIRA API CALLS


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





def main():
    print(GITHUB_REPOSITORY)

    issues = jira_get_jql("key", '"Develop repos[Labels]" in ("' + GITHUB_REPOSITORY  +  '") and ( "Master repos[Labels]" is EMPTY or "Master repos[Labels]" not in ("' + GITHUB_REPOSITORY  +  '"))')
    for issue in issues:
        print(issue["key"])

        # - set "Master version" 
        # - add new repo to Production repos and Tagged repos
        fields = {
#            "customfield_10074" : [{"set" : "Production" }],
            "customfield_10080" : [{"add" : GITHUB_REPOSITORY }]
            }
        jira_update_fields(issue["key"], fields)

            # Add a merge master comment to Jira
        jira_set_comment(issue["key"], "merged to master using repository: " + GITHUB_REPOSITORY)






if __name__ == "__main__":
    main()