import java.text.SimpleDateFormat

def FAIL_STATUS = false
def output = ""

pipeline {
    agent {
        kubernetes {
            yamlFile '.jenkins/jenkins_runner_manifest.yaml'
        }
    }
    stages {
        stage('Git') {
            steps {
                container('git') {
                    script {
                        sh "git config --global --add safe.directory ${WORKSPACE}"
                        sh "git fetch --depth 50 --no-tags --force https://github.com/foursquare/infraeng-weep.git +refs/heads/dev:refs/remotes/origin/dev"
                        env.COMMIT_HASH = sh(returnStdout: true, script: "git rev-parse --short HEAD").trim()
                        env.FULL_COMMIT_HASH = sh(returnStdout: true, script: "git rev-parse HEAD").trim()
                        env.COMMIT_DATE = sh(returnStdout: true, script: "git show -s --format=%cd --date=iso HEAD").trim()
                    }
                }
            }
        }
        stage('gofmt') {
            when {
                expression {
                    env.CHANGE_ID
                }
            }
            steps {
                container('go') {
                    script {
                        sh "git config --global --add safe.directory ${WORKSPACE}"
                        sh "apt update; apt install -y python3-pip; python3 -m pip install pre-commit"
                        sh "go install golang.org/x/tools/cmd/goimports@latest"
                        catchError(buildResult: 'SUCCESS', stageResult: 'FAILURE') {
                            def lintStatus = sh(returnStatus: true, script: "cd ${WORKSPACE}; pre-commit run --all-files | tee ${WORKSPACE}/pre-commit.out")
                            def lintOut = readFile("${WORKSPACE}/pre-commit.out")
                            output = """Go tests build result: \n``` ${lintOut} ```"""
                            sh "echo ${lintStatus}"
                            if (lintStatus != 0) {
                                FAIL_STATUS = true
                                throw new Exception("Nonzero Exit. Test Failure.")
                            }
                        }
                    }
                }
            }
        }
        stage('xgo-build') {
            when {
                expression {
                    ! env.CHANGE_ID
                }
            }
            steps {
                container('xgo') {
                    script {
                        sh "git config --global --add safe.directory ${WORKSPACE}"
                        // build all os targets for amd64 and arm64, output to /build
                        def date = new Date()
                        def sdf = new SimpleDateFormat("yyyy-MM-dd HH.mm.ss Z")
                        def buildTime = sdf.format(date).trim()
                        // def buildTime = "1234"
                        // sh "xgo -v -go 1.17 -targets='./amd64,./arm64' -out='weep' -dest='/build' -ldflags=\"-s -w -extldflags '-static' -X github.com/netflix/weep/pkg/metadata.Version=${env.BRANCH_NAME}-${env.COMMIT_HASH} -X github.com/netflix/weep/pkg/metadata.Commit=${env.COMMIT_DATE} -X github.com/netflix/weep/pkg/metadata.Date=${buildTime} \" /${WORKSPACE}; ls /build"
                        sh "xgo -v -go 1.17 -targets='./amd64,./arm64' -out='weep' -dest='/build' -ldflags=\"-s -w -extldflags '-static' -X 'github.com/netflix/weep/pkg/metadata.Version=foursquare_${env.BRANCH_NAME}-${env.COMMIT_HASH}' -X 'github.com/netflix/weep/pkg/metadata.Commit=${env.COMMIT_DATE}' -X 'github.com/netflix/weep/pkg/metadata.Date=${buildTime}' \" /${WORKSPACE}; ls /build"
                    }
                }
            }
        }
        stage('upload') {
            when {
                expression {
                    ! env.CHANGE_ID
                }
            }
            steps {
                container('aws-s3') {
                    script {
                        sh "aws s3 cp --recursive /build s3://fscloud-infra-public-resources/weep/${env.BRANCH_NAME}-${env.COMMIT_HASH}/"
                    }
                }
            }
        }
        stage('make-latest') {
            when {
                branch 'release'
                expression {
                    ! env.CHANGE_ID
                }
            }
            steps {
                container('aws-s3') {
                    script {
                        sh "aws s3 cp --recursive s3://fscloud-infra-public-resources/weep/${env.BRANCH_NAME}-${env.COMMIT_HASH} s3://fscloud-infra-public-resources/weep/latest/"
                        sh "echo \"${env.BRANCH_NAME}\\n ${env.FULL_COMMIT_HASH}\">version.txt && aws s3 cp version.txt  s3://fscloud-infra-public-resources/weep/latest/"
                    }
                }
            }
        }
    }
    post {
        always {
            script {
                if (FAIL_STATUS == true) {
                    currentBuild.result = 'FAILURE'
                }
                if (env.CHANGE_ID) {
                    if (FAIL_STATUS == true) {
                        output += """\n:x:"""
                    } else {
                        output += """\n:white_check_mark:"""
                    }
                    pullRequest.comment(output)
                }
            }
        }
    }
}
