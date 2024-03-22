package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

func main() {
	http.HandleFunc("/webhook", webhookHandler)
	log.Println("Server is running on :3001")
	if err := http.ListenAndServe(":3001", nil); err != nil {
		log.Fatal(err)
	}
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid method: "+r.Method, http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	jsonBody := string(body)
	var data map[string]interface{}
	//解析JSON字符串
	err = json.Unmarshal([]byte(jsonBody), &data)
	if err != nil {
		errorHandler(w, "Error parsing JSON:"+err.Error())
		return
	}
	//Repo类
	repository, ok := data["repository"].(map[string]interface{})
	if !ok {
		errorHandler(w, "Cannot convert repository to interface{}")
		return
	}
	//项目名
	repoName, ok := repository["name"].(string)
	if !ok {
		errorHandler(w, "Cannot convert repo_name to string")
		return
	}
	log.Println("repo_name:" + repoName)
	//用户名/项目名
	fullName, ok := repository["full_name"].(string)
	if !ok {
		errorHandler(w, "Cannot convert full_name to string")
		return
	}
	fullName = strings.ToLower(fullName)
	log.Println("full_name:" + fullName)
	//地址
	cloneUrl, ok := repository["clone_url"].(string)
	if !ok {
		errorHandler(w, "Cannot convert clone_url to string")
		return
	}
	log.Println("clone_url:" + cloneUrl)

	basePath := "/data/"
	repoPath := basePath + repoName
	//If the repository already exists, delete it first
	if _, err := os.Stat(repoPath); !os.IsNotExist(err) {
		err = os.RemoveAll(repoPath)
		log.Println("Delete exist folder")
		if err != nil {
			http.Error(w, "Error deleting existing repository: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	cmd := exec.Command("git", "clone", urlConverter(cloneUrl), repoPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, "Error cloning repository: "+err.Error()+"\n"+string(output), http.StatusInternalServerError)
		return
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write(output)
		go dockerBuildAndRun(w, repoName, fullName, repoPath)
		return
	}
}

func dockerBuildAndRun(w http.ResponseWriter,
	repoName string, fullName string, repoPath string) {
	dockerCmd := exec.Command("sh", "/opt/start.sh", repoName, fullName, repoPath)
	dockerCmdOutput, err := dockerCmd.CombinedOutput()
	if err != nil {
		errorHandler(w, string(dockerCmdOutput))
		return
	}
	log.Println(string(dockerCmdOutput))
}

// 把cloneUrl转为docker内地址
func urlConverter(url string) string {
	baseUrl := "http://gitea:3000/"
	path := strings.Split(url, "//")[1]
	// 使用/作为分隔符分割路径
	parts := strings.Split(path, "/")
	// 截取并拼接字符串
	result := baseUrl + strings.Join(parts[1:], "/")
	log.Println("localCloneUrl: " + result)
	return result
}

func errorHandler(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("[Error] " + message))
	log.Println("[Error] " + message)
}
