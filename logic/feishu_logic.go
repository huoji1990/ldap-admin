package logic

import (
	"context"
	"fmt"
	"strings"

	"github.com/eryajf/go-ldap-admin/config"
	"github.com/eryajf/go-ldap-admin/model"
	"github.com/eryajf/go-ldap-admin/public/client/feishu"

	"github.com/eryajf/go-ldap-admin/public/tools"
	"github.com/eryajf/go-ldap-admin/service/ildap"
	"github.com/eryajf/go-ldap-admin/service/isql"
	"github.com/gin-gonic/gin"
)

type FeiShuLogic struct {
}

// 通过飞书获取部门信息
func (d *FeiShuLogic) SyncFeiShuDepts(c *gin.Context, req interface{}) (data interface{}, rspError interface{}) {
	// 1.获取所有部门
	deptSource, err := feishu.GetAllDepts()
	if err != nil {
		return nil, tools.NewOperationError(fmt.Errorf("获取飞书部门列表失败：%s", err.Error()))
	}
	depts, err := ConvertDeptData(config.Conf.FeiShu.Flag, deptSource)
	if err != nil {
		return nil, tools.NewOperationError(fmt.Errorf("转换飞书部门数据失败：%s", err.Error()))
	}

	// 2.将远程数据转换成树
	deptTree := GroupListToTree(fmt.Sprintf("%s_0", config.Conf.FeiShu.Flag), depts)

	// 3.根据树进行创建
	err = d.addDepts(deptTree.Children)

	return nil, err
}

// 添加部门
func (d FeiShuLogic) addDepts(depts []*model.Group) error {
	for _, dept := range depts {
		err := d.AddDepts(dept)
		if err != nil {
			return tools.NewOperationError(fmt.Errorf("DsyncFeiShuDepts添加部门失败: %s", err.Error()))
		}
		if len(dept.Children) != 0 {
			err = d.addDepts(dept.Children)
			if err != nil {
				return tools.NewOperationError(fmt.Errorf("DsyncFeiShuDepts添加部门失败: %s", err.Error()))
			}
		}
	}
	return nil
}

// AddGroup 添加部门数据
func (d FeiShuLogic) AddDepts(group *model.Group) error {
	// 查询当前分组父ID在MySQL中的数据信息
	parentGroup := new(model.Group)
	err := isql.Group.Find(tools.H{"source_dept_id": group.SourceDeptParentId}, parentGroup)
	if err != nil {
		return tools.NewMySqlError(fmt.Errorf("查询父级部门失败：%s", err.Error()))
	}

	// 此时的 group 已经附带了Build后动态关联好的字段，接下来将一些确定性的其他字段值添加上，就可以创建这个分组了
	group.Creator = "system"
	group.GroupType = "cn"
	group.ParentId = parentGroup.ID
	group.Source = config.Conf.FeiShu.Flag
	group.GroupDN = fmt.Sprintf("cn=%s,%s", group.GroupName, parentGroup.GroupDN)

	if !isql.Group.Exist(tools.H{"group_dn": group.GroupDN}) {
		err = CommonAddGroup(group)
		if err != nil {
			return tools.NewOperationError(fmt.Errorf("添加部门: %s, 失败: %s", group.GroupName, err.Error()))
		}
	}
	return nil
}

// 根据现有数据库同步到的部门信息，开启用户同步
func (d FeiShuLogic) SyncFeiShuUsers(c *gin.Context, req interface{}) (data interface{}, rspError interface{}) {
	// 1.获取飞书用户列表
	staffSource, err := feishu.GetAllUsers()
	if err != nil {
		return nil, tools.NewOperationError(fmt.Errorf("获取飞书用户列表失败：%s", err.Error()))
	}
	staffs, err := ConvertUserData(config.Conf.FeiShu.Flag, staffSource)
	if err != nil {
		return nil, tools.NewOperationError(fmt.Errorf("转换飞书用户数据失败：%s", err.Error()))
	}
	// 2.遍历用户，开始写入
	for _, staff := range staffs {
		// 入库
		err = d.AddUsers(staff)
		if err != nil {
			return nil, tools.NewOperationError(fmt.Errorf("SyncFeiShuUsers写入用户失败：%s", err.Error()))
		}
	}

	return nil, nil
}

// AddUser 添加用户数据
func (d FeiShuLogic) AddUsers(user *model.User) error {
	// 根据角色id获取角色
	roles, err := isql.Role.GetRolesByIds([]uint{2})
	if err != nil {
		return tools.NewValidatorError(fmt.Errorf("根据角色ID获取角色信息失败:%s", err.Error()))
	}
	user.Roles = roles
	user.Creator = "system"
	user.Source = config.Conf.FeiShu.Flag
	user.Password = config.Conf.Ldap.UserInitPassword
	user.UserDN = fmt.Sprintf("uid=%s,%s", user.Username, config.Conf.Ldap.UserDN)

	// 根据 user_dn 查询用户,不存在则创建
	if !isql.User.Exist(tools.H{"user_dn": user.UserDN}) {
		// 获取用户将要添加的分组
		groups, err := isql.Group.GetGroupByIds(tools.StringToSlice(user.DepartmentId, ","))
		if err != nil {
			return tools.NewMySqlError(fmt.Errorf("根据部门ID获取部门信息失败" + err.Error()))
		}
		var deptTmp string
		for _, group := range groups {
			deptTmp = deptTmp + group.GroupName + ","
		}
		user.Departments = strings.TrimRight(deptTmp, ",")

		// 添加用户
		err = CommonAddUser(user, groups)
		if err != nil {
			return tools.NewOperationError(fmt.Errorf("添加用户: %s, 失败: %s", user.Username, err.Error()))
		}
	}
	return nil
}

// 订阅MQ飞书离职信息，并删除用户
func FeishuMqDelete() {
	c, err := rocketmq.NewPushConsumer(consumer.WithGroupName("GID_DEVOPS_LDAP"), // 消费者组
		consumer.WithNameServer([]string{"127.0.0.1:9876"}), // nameserver
		consumer.WithRetry(2),
		consumer.WithConsumeFromWhere(consumer.ConsumeFromLastOffset))
	if err != nil {
		fmt.Println("消费者实例创建失败")
	}
	c.Subscribe("TOPIC_PROD_FEISHU_EVENT", consumer.MessageSelector{}, func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for i := range msgs {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(msgs[i].Body), &data); err == nil {
				if isql.User.Exist(tools.H{"source_union_id": fmt.Sprintf("%s_%s", config.Conf.FeiShu.Flag, data["unionId"])}) {
					user := new(model.User)
					isql.User.Find(tools.H{"source_union_id": fmt.Sprintf("%s_%s", config.Conf.FeiShu.Flag, data["unionId"])}, user)
					// ldap删除用户
					ildap.User.Delete(user.UserDN)
					// Mysql删除用户
					isql.User.Delete([]uint{user.ID})
				}
			}

		}

		return consumer.ConsumeSuccess, nil // 回调函数
	})
	c.Start()
}

// 依据飞书事件删除用户
func FeishuEventDelete(unionid string) {
	if isql.User.Exist(tools.H{"source_union_id": fmt.Sprintf("%s_%s", config.Conf.FeiShu.Flag, unionid)}) {
		user := new(model.User)
		isql.User.Find(tools.H{"source_union_id": fmt.Sprintf("%s_%s", config.Conf.FeiShu.Flag, unionid)}, user)
		// ldap删除用户
		ildap.User.Delete(user.UserDN)
		// Mysql删除用户
		isql.User.Delete([]uint{user.ID})
	}
}
