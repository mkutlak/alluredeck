// Jenkins declarative pipeline example: run tests and publish results to Allure Docker Service.
//
// Required plugins:
//   - HTTP Request: https://plugins.jenkins.io/http_request/
//   - Pipeline Utility Steps: https://plugins.jenkins.io/pipeline-utility-steps/
//
// If security is enabled, store credentials in Jenkins Credentials Store as
// "Username with password" and reference via the ALLURE_CREDENTIALS_ID variable.

import groovy.json.JsonOutput
import java.net.URLEncoder

// --- Configuration ---
def allureServer    = 'http://your-allure-host:5050'  // Change to your Allure service URL
def projectId       = 'default'                        // Change to your project ID
def resultsPattern  = 'allure-results/*'               // Glob pattern for result files
def credentialsId   = ''                               // Jenkins credentials ID (leave empty if no auth)

// --- Helpers ---
String buildResultsJson(String pattern) {
    def results = []
    findFiles(glob: pattern).each { file ->
        def b64 = readFile(file: file.path, encoding: 'Base64')
        if (!b64.trim().isEmpty()) {
            results << [file_name: file.name, content_base64: b64]
        } else {
            echo "Skipping empty file: ${file.path}"
        }
    }
    return JsonOutput.toJson([results: results])
}

String getAccessToken(String serverUrl, String user, String pass) {
    def body = JsonOutput.toJson([username: user, password: pass])
    def response = httpRequest(
        url           : "${serverUrl}/login",
        httpMode      : 'POST',
        contentType   : 'APPLICATION_JSON',
        requestBody   : body,
        validResponseCodes: '200'
    )
    def json = readJSON(text: response.content)
    return json.data.access_token
}

void sendResults(String serverUrl, String projectId, String resultsJson, String token) {
    def headers = [[name: 'Content-Type', value: 'application/json']]
    if (token) headers << [name: 'Authorization', value: "Bearer ${token}"]

    httpRequest(
        url              : "${serverUrl}/send-results?project_id=${projectId}",
        httpMode         : 'POST',
        customHeaders    : headers,
        requestBody      : resultsJson,
        consoleLogResponseBody: true,
        validResponseCodes: '200'
    )
}

String generateReport(String serverUrl, String projectId, String execName, String execFrom, String execType, String token) {
    def query = "project_id=${projectId}"
    if (execName) query += "&execution_name=${URLEncoder.encode(execName, 'UTF-8')}"
    if (execFrom) query += "&execution_from=${URLEncoder.encode(execFrom, 'UTF-8')}"
    if (execType) query += "&execution_type=${execType}"

    def headers = [[name: 'Content-Type', value: 'application/json']]
    if (token) headers << [name: 'Authorization', value: "Bearer ${token}"]

    def response = httpRequest(
        url              : "${serverUrl}/generate-report?${query}",
        httpMode         : 'POST',
        customHeaders    : headers,
        consoleLogResponseBody: true,
        validResponseCodes: '200'
    )
    def json = readJSON(text: response.content)
    return json.data.report_url
}

// --- Pipeline ---
pipeline {
    agent any

    options {
        disableConcurrentBuilds()
        buildDiscarder(logRotator(numToKeepStr: '20'))
    }

    stages {
        stage('Run Tests') {
            steps {
                // Replace with your actual test command, e.g.:
                //   sh 'pytest --alluredir=allure-results'
                //   sh 'mvn test'
                //   sh 'npx playwright test'
                warnError('Some tests failed') {
                    echo 'Replace this with your test command'
                }
            }
        }

        stage('Send Results to Allure') {
            steps {
                script {
                    def token = ''
                    if (credentialsId) {
                        withCredentials([usernamePassword(
                            credentialsId: credentialsId,
                            usernameVariable: 'ALLURE_USER',
                            passwordVariable: 'ALLURE_PASS'
                        )]) {
                            token = getAccessToken(allureServer, env.ALLURE_USER, env.ALLURE_PASS)
                        }
                    }

                    def resultsJson = buildResultsJson(resultsPattern)
                    sendResults(allureServer, projectId, resultsJson, token)

                    def reportUrl = generateReport(
                        allureServer, projectId,
                        "Build #${env.BUILD_NUMBER}",
                        env.BUILD_URL,
                        'jenkins',
                        token
                    )
                    echo "Allure Report: ${reportUrl}"
                    currentBuild.description = "<a href='${reportUrl}'>Allure Report</a>"
                }
            }
        }
    }

    post {
        always {
            echo 'Pipeline finished.'
        }
    }
}
