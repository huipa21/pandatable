# PandaTable user guide / 用户指南

[English](#english) | [简体中文](#简体中文)

## English

### 1. Sign in and set your preferences

Ask an administrator for an account, then sign in with your username or email.
You may be required to change your temporary password before continuing. There
is no self-registration. Use **Profile** to change your display name, language,
theme, or timezone and save your changes; use **Change password** to update your
password. The language and theme controls in the header also change the display.
If SMTP is configured, **Forgot password?** sends a reset link to the email
address on your account. The link expires after 30 minutes and can be used
once; changing the password signs out all sessions. In **Profile**, optionally
enable mention emails; in-app notifications remain available.

### 2. Navigate and share

PandaTable organizes data as workspaces, books, tables, and views. In **Workspace
navigation**, choose **New workspace**, open it and choose **New book**, then
open the book and choose **New table**. A new table has a default grid and a
display field named Name or 名称 according to the creator's profile language.
On a narrow screen, open the navigation drawer first.

An owner can use **Share** on a workspace, book, or table to find a user or
group, choose **Read**, **Write**, or **Owner** access, and review and confirm
the change.
Readers can view data; writers can edit rows; owners can manage fields and
sharing. Access inherited from a parent cannot be reduced on a child.
Administrators do not automatically have access to workspace data. If a
control or resource is unavailable, contact its owner.

### 3. Work with rows and fields

Open a table's grid, use **Add row**, and edit cells with the appropriate
editor. Invalid values are reported and not saved. Owners can manage field
definitions; use **Fields** to control which columns are visible. Attachment
fields support file selection and drag-and-drop; the administrator's upload
limit applies.

On desktop, copy and paste cell ranges, use Ctrl+D to fill down, and use Ctrl+Z
or Ctrl+Y to undo or redo. An undo can ask for confirmation if another user
changed the data. On a narrow screen, rows appear as cards; tap a card to open
the row editor or use the add-row control to create a row.

### 4. Use views and forms

Select an existing view or enter a view name, choose **Personal** or **Shared**
and a **Grid**, **Form**, **Kanban**, or **Gallery** type, then choose **Create view**.
Readers can create personal views; shared views require write access. Owners can
lock shared views. If you cannot save changes to a shared view, you can apply
temporary changes and use **Save current changes as a personal view**.

Grid controls include **Filter rows**, **Sort rows**, search across visible
fields, grouping, highlighting, and footer aggregates. In a form view, an owner
can configure the fields and labels; users with write access can submit new
rows, while readers cannot submit.

In Kanban settings, choose a single-select or user field for the columns and
select the card fields. Unassigned rows have their own column; for user
grouping, search for a table user to add an empty destination column. Use the
column and card page controls to see more items. Writers can drag a card or
choose its **Move card** control and a destination; readers can open cards without
moving them. Other participants' changes refresh the board.

In Gallery settings, choose an attachment field for the cover and select the
card fields. The first attachment with an image thumbnail becomes the cover;
cards without an available image show a placeholder. Use the page controls to
see more cards, and choose **Open row** to inspect or edit a record according
to your access. Other participants' row changes refresh the gallery.

### 5. Collaborate

Table presence shows other participants and their focused cells. Open a row to
see **Comments** and **History**. In a comment, type `@`, select an accessible
user, and choose **Post comment**. The recipient can open the notification bell
to navigate to the comment. Edits are synchronized while connected; when two
people save the same cell, the last saved value wins.

### 6. Import and export

To create a table from a CSV or XLSX file, open a book and choose **Import as a
new table**. To append to an existing table, open it and choose **Import into
this table**; write access is required. Select the file, choose **Analyze
file**, review the detected columns, types, or mappings, and choose **Start
import**. Review the result and download the invalid-cell report when offered.

From a grid, **Export → CSV (current view)** downloads the visible fields and
respects the current filter, sort, and search. A **PandaTable XLSX package**
can also be exported and imported, but it is **not a complete backup**:
nonempty attachment identities and contents are not recovered, distinct
timestamps may collapse to the same value, and view locks are not preserved.
Review link choices and any reported dependencies after importing a package.

### 7. Trash and backups

Deleted books, tables, views, and fields can be restored by an owner from
**Trash** before their retention period expires. Review the restore prompts,
including any optional field restoration.

Administrators can open **Backups**, choose **Create backup**, wait for the
status to finish, and **Download** the archive for safe storage. Restoring a
backup requires an operator using the documented server CLI; a package export
is not a substitute for a backup. See the [deployment guide](13-deployment.md)
for operational procedures.

Administrators can open **Settings → Email (SMTP)**, configure and save the
mail server, then send a test message. Leave credential fields blank to retain
stored values, or select their removal controls to clear them. An SMTP password
requires the operator to provision the encryption key described in the deployment
guide.

## 简体中文

### 1. 登录和设置偏好

请向管理员获取账号，然后使用用户名或邮箱登录。首次使用临时密码时，可能需要先
修改密码。系统不提供自助注册。在**个人资料**中修改显示名称、语言、主题或时区并
保存；使用**修改密码**更新密码。页眉中的语言和主题控件也可以切换显示。
如果已配置 SMTP，可通过**忘记密码？**向账号邮箱申请重置链接。链接 30 分钟内
有效且只能使用一次；修改密码后所有会话都会退出。在**个人资料**中可选择启用
提及邮件，应用内通知仍然可用。

### 2. 导航和共享

PandaTable 按工作区、工作簿、数据表和视图组织数据。在**工作区导航**中选择
**新建工作区**，打开后选择**新建工作簿**，再打开工作簿并选择**新建数据表**。
新表默认包含一个网格视图和显示字段，其名称根据创建者的个人资料语言为 Name 或
名称。在窄屏上，请先打开导航抽屉。

所有者可在工作区、工作簿或数据表上使用**共享**，查找用户或用户组，选择**只读**、
**读写**或**所有者**权限，并在审核后确认更改。读取者可以查看数据；写入者可以编辑行；
所有者可以管理字段和共享。从上级继承的权限不能在下级降低。管理员不会自动获得
工作区数据的访问权限。如果看不到某个控件或资源，请联系其所有者。

### 3. 使用行和字段

打开数据表的网格，选择**添加行**，并使用相应的编辑器修改单元格。无效值会显示
错误且不会保存。所有者可以管理字段定义；使用**字段**控制显示哪些列。附件
字段支持选择文件和拖放；上传大小受管理员设置的限制。

在桌面端，可以复制粘贴单元格区域、使用 Ctrl+D 向下填充，以及使用 Ctrl+Z 或
Ctrl+Y 撤销或重做。如果其他用户已更改数据，撤销时可能需要确认。在窄屏上，行以
卡片显示；点击卡片可打开行编辑器，也可以使用添加行控件新建行。

### 4. 使用视图和表单

选择已有视图，或输入视图名称，选择**个人**或**共享**以及**网格**、**表单**、
**看板**或**画廊**类型，然后选择**创建视图**。读取者可以创建个人视图；共享视图需要写入
权限。所有者可以锁定共享视图。如果无法保存共享视图的更改，可以临时调整并选择
**将当前更改保存为个人视图**。

网格控件包括**筛选行**、**排序行**、搜索可见字段、分组、高亮和底部汇总。在
表单视图中，所有者可以配置字段和标签；有写入权限的用户可以提交新行，读取者
不能提交。

在看板设置中，选择单选或用户字段作为列，并选择卡片字段。未分配的行有单独的列；
按用户分组时，可以搜索数据表用户来添加空的目标列。使用列和卡片的翻页控件查看更多
项目。有写入权限的用户可以拖动卡片，或选择卡片的**移动卡片**控件和目标；读取者可以
打开卡片，但不能移动。其他参与者的更改会刷新看板。

在画廊设置中，选择附件字段作为封面，并选择卡片字段。第一个带有图片缩略图的附件
将作为封面；没有可用图片的卡片会显示占位内容。使用翻页控件查看更多卡片，并选择
**打开行**，根据权限查看或编辑记录。其他参与者的行更改会刷新画廊。

### 5. 协作

数据表的在线状态会显示其他参与者及其聚焦的单元格。打开一行可查看**评论**和
**历史记录**。在评论中输入 `@`，选择有访问权限的用户，然后选择**发表评论**。
接收者可以打开通知铃铛，跳转到该评论。连接正常时编辑会同步；两人保存同一单元格
时，以最后保存的值为准。

### 6. 导入和导出

要从 CSV 或 XLSX 文件创建数据表，请打开工作簿并选择**导入为新表**。要向已有表
追加数据，请打开该表并选择**导入到此表**；这需要写入权限。选择文件，点击
**分析文件**，检查检测到的列、类型或映射，然后选择**开始导入**。检查结果，并在
提供报告时下载无效单元格报告。

在网格中，**导出 → CSV（当前视图）**会下载可见字段，并遵循当前筛选、排序和
搜索。也可以导出和导入 **PandaTable XLSX 数据包**，但它**不是完整备份**：
非空附件的身份和内容无法恢复，不同时间戳可能合并为同一值，视图锁定状态也不会
保留。导入数据包后，请检查关联选项和报告的依赖问题。

### 7. 回收站和备份

在保留期限内，所有者可以从**回收站**恢复已删除的工作簿、数据表、视图和字段。
请检查恢复提示，包括可选的字段恢复。

管理员可以打开**备份**，选择**创建备份**，等待状态完成，然后**下载**归档并
妥善保存。恢复备份需要运维人员使用文档中的服务器命令行工具；数据包导出不能
代替备份。运维流程请参阅[部署指南](13-deployment.md)。

管理员可以打开**设置 → 电子邮件（SMTP）**，配置并保存邮件服务器，然后发送测试
邮件。凭据字段留空可保留原值，也可以选择移除控件清除原值。保存 SMTP 密码前，
运维人员需按部署指南配置加密密钥。
