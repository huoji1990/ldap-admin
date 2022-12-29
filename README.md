#编译命令：

git clone 代码
cd ldap-admin-server
docker build -t ldap-admin-server.

#docker运行命令：

docker run  -p 8888:8888 ldap-admin-server

#说明：

此服务为LDAP系统的后端部分，需提前部署OpenLDAP系统，然后在config.yml中进行相关配置