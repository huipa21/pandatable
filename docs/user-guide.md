# PandaTable user guide / 用户指南

[English](#english) | [简体中文](#简体中文)

## English

### 1. Sign in and set your preferences

Ask an administrator for an account, then sign in with your username or email.
You may be required to change your temporary password before continuing. There
is no self-registration. Open the avatar menu at the bottom of the sidebar (or
in the header when the sidebar is hidden) for **Profile**, **Change password**,
**Trash**, notifications, the admin pages (administrators only) and **Sign
out**. Use **Profile** to change your display name, language,
theme, or timezone and save your changes; use **Change password** to update your
password. Use the translate icon in the header to choose **English** or
**简体中文**; the menu shows each language's native and localized name. The
theme control is next to it.
If SMTP is configured, **Forgot password?** sends a reset link to the email
address on your account. The link expires after 30 minutes and can be used
once; changing the password signs out all sessions. In **Profile**, optionally
enable mention emails; in-app notifications remain available.

### 2. Navigate and share

PandaTable organizes data as workspaces (a company or program), books (a project
whose tables can relate to each other), tables, and views (filtered or sorted
tables). Signing in opens the **Workspaces** dashboard. New accounts start with
a workspace named **Default**, which its owner can rename. The dashboard lists
every workspace available to you, with a search field and each workspace's
icon, name and optional description. Recent workspaces appear first; searching
matches names and descriptions. Use **New workspace** to create a workspace
with an optional icon and description. In a workspace row's **⋯** menu, choose
**Set as default** to change your preferred workspace; visiting another
workspace does not change that preference.

Open a workspace and, if you are its owner, choose **⋮** next to its name in the
sidebar and select **New book**. Book creation and editing can include an
optional description of up to 500 code points; non-empty descriptions appear
below the name on the workbook card. Workspace pages show visible books as
cards ordered by recent opens, then server order. Use the header quick-search
(Ctrl+K) to find workspaces, books, tables, or views; there is no separate
workspace-page search field. Each book keeps the same icon for everyone who
can see it. A new table has a default grid and a display field named Name or
名称 according to the creator's profile language.
Select a book to show its tables, select a table name to open its default view,
or activate the table icon to list its views. On workspace pages the sidebar
links to **Workspaces** and shows the current workspace. On workbook, table,
view, or row pages, it also shows only the current workbook, its tables, and
their views; workbook trees start collapsed except for the active route. Use
**Workspaces** or the workbook cards to switch context. A
path-only workspace is context for shared descendants, not an eligible default
or an invitation to manage its structure. Workspace owners can use the
sidebar's **⋮** menu to edit the workspace; book owners can use the book's
**Edit** action to change its name or description. Breadcrumbs start with the
current workspace, with no **Workspaces** root; **/home** has no breadcrumb.
The arrow next to the logo hides the sidebar; on a narrow screen it opens the
navigation drawer.

An owner can use **Share** in a workspace row's or resource's **⋯** action
menu to find a user or group, choose **Read**, **Write**, or **Owner** access,
and review and confirm the change.
Workspaces are private by default: only the owner and people granted access can
see them and their contents. Access granted on a workspace or book is inherited
by everything beneath it; to share only one book, grant access on that book.
Readers can view data; writers can edit rows; owners can manage fields and
sharing. Access inherited from a parent cannot be reduced on a child.
Administrators do not automatically have access to workspace data. If a
control or resource is unavailable, contact its owner.

### 3. Work with rows and fields

Open a table's grid and edit cells with the appropriate editor. On desktop, the
full-width plus row after the last row creates one row immediately using
defaults. To fill in values first, choose **New** at the right of the toolbar.
The **New entry** form has inline labels and editors, a field-name search, and
a **Save** icon in its upper-right corner; system fields are omitted. Untouched
fields use their defaults, and closing the draft creates nothing. If a required
field has no default, use the form instead of the plus. Invalid values are
reported and not saved. If write access is revoked while the form is open, it
reports the permission change and prevents saving. On desktop, **Fields**
searches the field list and controls which columns are visible; choose **System fields** to include system
columns. Owners can add a field with the plus button
in the grid header or **Add field** in the field menu, choose its type, and
optionally set a default value and description. The searchable type list also
includes relations, lookups, rollups and formulas. In **Fields**, each field is
a compact tile: drag its name to change the order of the current grid (owners
also save the table's field order), use its checkbox to show or hide it, or
open its **⋯** menu to edit, move or delete it. **Edit** uses the same layout
as **Add field**, including the display-field option under advanced settings;
owners can change a field's type there when a safe conversion
exists. For single-select and multi-select fields, new options receive
different preset colors automatically. Choose a swatch to change a color, and
drag an option's handle, press Alt+Up/Down on it, or use its move controls to
reorder the options. Attachment fields support file selection and
drag-and-drop; the administrator's upload limit applies. When creating or
editing a checkbox field, choose its **Icon** (including a heart or star).
Date fields provide a calendar; time and date-time fields provide pickers with
confirmation. Date-time values use the field's fixed timezone or the viewer's
timezone, as configured.

Hover or focus a column header and open its arrow menu to hide, sort, filter,
or group by that field. Owners can also edit its settings or description,
insert a field beside it, duplicate its schema without copying row values, set
the display field, or delete it after confirmation. System and link fields
cannot be duplicated, and the display field cannot be hidden or deleted.
Changes that cannot be saved to the current view remain local.

In the desktop grid, click a text, number or date cell once to edit it (double-click,
Enter or F2 also work). Leaving the cell saves a valid value; an invalid value
stays in the editor with its error, and Escape cancels. Click a checkbox to
toggle it or click a select cell to choose an option. Multi-select choices stay
open while you make changes; leave the editor or press Enter to save them, or
press Escape to cancel.

On desktop, copy and paste cell ranges, use Ctrl+D to fill down, and use Ctrl+Z
or Ctrl+Y to undo or redo. An undo can ask for confirmation if another user
changed the data. On a narrow screen, rows appear as cards; tap a card to open
the row editor or use the single add-row control to open the creation form.

### 4. Use views and forms

Expand a table in the sidebar to select a view. To create one, open the table's
**⋯** menu and choose **Create view**; enter a name, choose **Personal** or
**Shared** and a **Grid**, **Form**, **Kanban**, or **Gallery** type, then
confirm. The new view appears under that table and opens automatically.
Readers can create personal views; shared views require write access. Open a
view's **⋯** menu for its permitted rename, duplicate, move, and delete actions;
owners can also lock or unlock a shared view. If you cannot save changes to a
shared view, you can apply temporary changes and use **Save current changes as a
personal view** on the table page.

The desktop grid toolbar provides **Filter**, **Group**, **Sort**, **Highlight**,
and **Format** controls. Format contains row height, column width, order, and
freezing; **Fields** controls visibility. **Search**, immediately left of
**New**, searches all visible searchable fields or a selected field. With
selected rows, writers can directly **Edit** or **Delete** them; bulk Edit
applies one typed value to the selected rows, and
Delete asks for confirmation. The summary bar stays at the bottom of the grid.
Hover or focus a column's summary cell (or tap it on a touch device) to choose
an aggregate, including percentage operations. Unconfigured summaries are
hidden; choosing **None** clears a saved summary. These controls are no longer
under a More menu. In a form view, an owner can configure the fields and labels;
users with write access can submit new rows, while readers cannot submit.

In an editable view, use **Highlight** to add up to 20 independent rules. Each
rule has a condition, a color, and a target of the whole row or selected cells.
Rules are checked from top to bottom: the first match wins independently for
each row and each cell, and a cell match overrides the row color. Drag a rule
heading or use **Move up** and **Move down** to reorder; incomplete drafts stay
local until complete.

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

The grid's **#** column is the row position, not its unique database ID (hidden
by default). Use its expand icon to open a row. The first data column supplies
the popup title. Search fields by name or value, or expand the popup to fill only
the table area. Click a value to edit it. Up/down navigate in view order
(Kanban uses displayed cards). Finish an active edit before navigating.
**Duplicate row** creates an unsaved draft: review unique values and choose
the **Save** icon to insert it. System, calculated and linked fields are not
copied. Closing the draft inserts nothing. **Delete row** asks for confirmation
and supports Undo. Comments/history icons open overlays; their close button or
Escape returns to the row fields without discarding an active field edit.

Table presence shows other participants and their focused cells. Open a row to
see **Comments** and **History**. In a comment, type `@`, select an accessible
user, and choose **Post comment**. The recipient can open notifications from the
avatar menu (its badge shows the unread count) to navigate to the comment. Edits are synchronized while connected; when two
people save the same cell, the last saved value wins.

### 6. Import and export

To create a table from an XLSX file, open a book and choose **Import as a
new table**. To append to an existing table, choose **Import** in its toolbar
to open **Import into this table**; write access is required. Select the file,
choose **Analyze file**, review the detected columns, types, or mappings, and
choose **Start import**. Review the result and download the invalid-cell report
when offered.

From a table or view, open **Import/Export** and choose **Import** or **Export**.
The exported XLSX contains the full table and shared views, with a visible
`_meta` sheet describing field types and a hidden sheet for view metadata.
Plain XLSX files are also supported. CSV import and current-view CSV export
remain available through the API, not the user interface.

An XLSX package is **not a complete backup**: attachment file contents are
not embedded, Excel timestamps can lose precision, and view locks are not
preserved. An attachment can be relinked only on the same PandaTable instance
when the source file still exists and you can read its source table; otherwise
its name appears in the issues report. Relations are matched to accessible
targets in the same book; unmatched or ambiguous values are reported. Review
the import report and any dependencies after importing.

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
修改密码。系统不提供自助注册。侧边栏底部的头像菜单（侧边栏隐藏时位于页眉）中
包含**个人资料**、**修改密码**、**回收站**、通知、管理页面（仅管理员）和**退出登录**。
在**个人资料**中修改显示名称、语言、主题或时区并
保存；使用**修改密码**更新密码。点击页眉中的翻译图标可选择**English**或**简体中文**；
菜单会显示语言的本地名称和当前界面语言名称。主题控件位于旁边。
如果已配置 SMTP，可通过**忘记密码？**向账号邮箱申请重置链接。链接 30 分钟内
有效且只能使用一次；修改密码后所有会话都会退出。在**个人资料**中可选择启用
提及邮件，应用内通知仍然可用。

### 2. 导航和共享

PandaTable 按工作区（公司或项目群）、工作簿（一个项目，其中的数据表可以相互
关联）、数据表和视图（经过筛选或排序的数据表）组织数据。登录后会打开**工作区**
仪表板。新账户最初有一个名为 **Default** 的工作区，所有者可将其重命名。仪表板列出
你可访问的所有工作区，并提供搜索框；每行显示图标、名称和可选描述。最近打开的工作区
排在前面，搜索可匹配名称和描述。选择**新建工作区**可创建工作区，并可选填图标和描述。
在工作区行的 **⋯** 菜单中选择**设为默认工作区**可更改首选工作区；仅访问其他工作区
不会更改此设置。

打开工作区后，工作区所有者可在侧边栏工作区名称旁选择 **⋮**，再选择**新建工作簿**。
创建或编辑工作簿时可选填最多 500 个代码点的描述；非空描述会显示在工作簿卡片名称下方。
工作区页面按最近打开时间及服务器顺序显示可见工作簿卡片。可使用页眉快速搜索（Ctrl+K）
查找工作区、工作簿、数据表或视图；工作区页面不再单独提供搜索框。每个工作簿对所有可见用户显示相同
的图标。新表默认包含
一个网格视图和显示字段，其名称根据创建者的个人资料语言为 Name 或名称。选择工作簿
可查看其数据表，选择数据表名称可打开默认视图，点击数据表图标可列出其视图。在工作区页面，
侧边栏链接到**工作区**并显示当前工作区。在工作簿、数据表、视图或行页面，侧边栏还会显示
当前工作簿及其数据表和视图；除当前路由所需的上级节点外，工作簿树默认折叠。切换工作区请使用
**工作区**链接或工作簿卡片。仅路径工作区只为共享的下级资源提供上下文，不能设为默认工作区，
也不能管理其结构。工作区所有者可通过侧边栏 **⋮** 菜单编辑工作区；工作簿所有者可使用工作簿的
**编辑**操作修改名称或描述。面包屑从当前工作区开始，不包含**工作区**根项；**/home** 不显示面包屑。
页眉搜索按钮（Ctrl+K）可按名称跳转到任意工作区、工作簿、数据表或视图。
徽标旁的箭头可隐藏侧边栏；在窄屏上它会打开导航抽屉。

工作区默认私有：只有所有者和获授权的用户可查看工作区及其内容。在工作区或工作簿授予的访问权限
会继承到其下级资源；若只需共享单个工作簿，请在该工作簿上授予访问权限。
所有者可在工作区行或资源的 **⋯** 操作菜单中选择**共享**，查找用户或用户组，选择**只读**、
**读写**或**所有者**权限，并在审核后确认更改。读取者可以查看数据；写入者可以编辑行；
所有者可以管理字段和共享。从上级继承的权限不能在下级降低。管理员不会自动获得
工作区数据的访问权限。如果看不到某个控件或资源，请联系其所有者。

### 3. 使用行和字段

打开数据表的网格，并使用相应的编辑器修改单元格。在桌面端，最后一行下方横跨表格的
加号会使用默认值直接创建一行；要先填写内容，请选择工具栏右侧的**新建**。
**新建记录**表单将标签和编辑器并排显示，可按字段名搜索，并通过右上角的**保存**图标提交；
系统字段不会显示。未编辑的字段使用默认值；关闭草稿不会创建行。如果必填字段没有
默认值，请使用表单而不是加号。无效值会显示错误且不会保存。如果表单打开时写入权限被撤销，
表单会提示权限变更并禁止保存。在桌面端，使用**字段**
搜索字段并控制显示哪些列；选择**系统字段**
可显示系统列。所有者可以通过网格表头的加号或字段菜单中的**添加字段**新建字段，
选择类型，并按需设置默认值和说明。可搜索的类型列表也包含关联、查找、汇总和公式。
在**字段**中，每个字段显示为紧凑的磁贴：拖动字段名可调整当前网格的列顺序（所有者
同时保存数据表的字段顺序），用复选框显示或隐藏它，或打开 **⋯** 菜单编辑、移动或删除。
**编辑**与**添加字段**使用相同布局，高级设置中可设为显示字段；存在安全转换时，所有者可在其中更改字段类型。
单选和多选的新选项会自动分配预设颜色；可通过色块更改颜色，拖动手柄、按 Alt+上/下方向键
或使用移动控件调整顺序。附件字段支持选择文件和拖放；上传大小受管理员设置的限制。
创建或编辑复选框字段时，可在**图标**中选择爱心、星形等样式。日期字段提供日历；
时间和日期时间字段提供可确认的选择器。日期时间按字段配置的固定时区或查看者时区显示。

将鼠标悬停在列标题上或聚焦标题，然后打开箭头菜单，可按该字段隐藏、排序、筛选或分组。
所有者还可以编辑字段设置或说明、在其旁边插入字段、仅复制字段结构而不复制行值、
设为显示字段，或在确认后删除。系统字段和关联字段不能复制，显示字段不能隐藏或删除。
无法保存到当前视图的更改只在本地生效。

在桌面网格中，单击文本、数字或日期单元格即可编辑（双击、Enter 或 F2 同样可用）。
离开单元格时会保存有效值；无效值会连同错误提示保留在编辑器中，按 Escape 取消。
单击复选框可切换其状态，单击选择字段单元格可选择选项。多选选项在修改期间保持打开；
离开编辑器或按 Enter 保存，按 Escape 取消。

在桌面端，可以复制粘贴单元格区域、使用 Ctrl+D 向下填充，以及使用 Ctrl+Z 或
Ctrl+Y 撤销或重做。如果其他用户已更改数据，撤销时可能需要确认。在窄屏上，行以
卡片显示；点击卡片可打开行编辑器，也可以使用唯一的添加行控件打开创建表单。

### 4. 使用视图和表单

展开侧边栏中的数据表并选择视图。要创建视图，请打开数据表的 **⋯** 菜单，选择
**创建视图**，输入名称，选择**个人**或**共享**以及**网格**、**表单**、**看板**或
**画廊**类型，然后确认。新视图会显示在该数据表下并自动打开。读取者可以创建个人
视图；共享视图需要写入权限。打开视图的 **⋯** 菜单，可执行有权限的重命名、复制、
移动和删除操作；所有者还可以锁定或解锁共享视图。如果无法保存共享视图的更改，可以
临时调整，并在数据表页面选择**将当前更改保存为个人视图**。

桌面网格工具栏提供**筛选**、**分组**、**排序**、**高亮**和**格式**控件。**格式**
包含行高、列宽、顺序和冻结；**字段**控制列的可见性。**新建**左侧的**搜索**可搜索
所有可见且支持搜索的字段，也可选择单个字段。
选择行后，写入者可以直接**编辑**或**删除**所选行；批量编辑会将一个类型化的值应用到
所选行，删除前会要求确认。汇总栏固定在网格底部；将鼠标悬停或聚焦在列汇总单元格上
（触屏可直接点击），即可选择汇总方式，包括百分比运算。未配置的汇总默认隐藏；
选择**无**可清除已保存的汇总。
这些控件不再位于“更多”菜单中。在表单视图中，所有者可以配置字段
和标签；有写入权限的用户可以提交新行，读取者不能提交。

在可编辑的视图中，使用**高亮**添加最多 20 条独立规则。每条规则都有条件、颜色和目标
（整行或所选单元格）。规则从上到下依次检查，每一行和每个单元格分别使用第一条匹配的
规则；单元格匹配会覆盖行颜色。拖动规则标题，或使用**上移**和**下移**调整顺序；未完成的
草稿会保留在本地，直到填写完整。

在看板设置中，选择单选或用户字段作为列，并选择卡片字段。未分配的行有单独的列；
按用户分组时，可以搜索数据表用户来添加空的目标列。使用列和卡片的翻页控件查看更多
项目。有写入权限的用户可以拖动卡片，或选择卡片的**移动卡片**控件和目标；读取者可以
打开卡片，但不能移动。其他参与者的更改会刷新看板。

在画廊设置中，选择附件字段作为封面，并选择卡片字段。第一个带有图片缩略图的附件
将作为封面；没有可用图片的卡片会显示占位内容。使用翻页控件查看更多卡片，并选择
**打开行**，根据权限查看或编辑记录。其他参与者的行更改会刷新画廊。

### 5. 协作

网格的 **#** 列表示行位置，不是默认隐藏的唯一数据库 ID。点击展开图标可打开行，
首个数据列用作弹窗标题。可按字段名称或值搜索，或仅在数据表区域内全屏；点击字段值
即可编辑。上下箭头按视图顺序导航（看板仅导航当前显示的卡片），导航前请完成
当前编辑。**复制行**创建未保存的草稿，检查唯一字段后点击**保存**图标才会插入；
系统、计算和关联字段不会复制，关闭草稿不会插入。**删除行**需要确认并支持撤销。
评论和历史记录以覆盖层显示，点击关闭或按 Escape 返回字段，并保留当前字段草稿。

数据表的在线状态会显示其他参与者及其聚焦的单元格。打开一行可查看**评论**和
**历史记录**。在评论中输入 `@`，选择有访问权限的用户，然后选择**发表评论**。
接收者可以从头像菜单（徽标显示未读数量）打开通知，跳转到该评论。连接正常时编辑会同步；两人保存同一单元格
时，以最后保存的值为准。

### 6. 导入和导出

要从 XLSX 文件创建数据表，请打开工作簿并选择**导入为新表**。要向已有表
追加数据，请在该表的工具栏选择**导入**，打开**导入到此表**；这需要写入权限。选择文件，点击
**分析文件**，检查检测到的列、类型或映射，然后选择**开始导入**。检查结果，并在
提供报告时下载无效单元格报告。

在数据表或视图中，打开工具栏中的**导入/导出**，选择**导入**或**导出**。
导出的 XLSX 包含完整数据表和共享视图、描述字段类型的可见 `_meta` 工作表，以及保存
视图元数据的隐藏工作表；也支持普通 XLSX 文件。CSV 导入及当前视图的 CSV 导出仍可通过
API 使用，但不在用户界面中提供。

XLSX 数据包**不是完整备份**：附件内容不会嵌入，Excel 时间戳可能损失精度，视图锁定
状态也不会保留。只有在同一 PandaTable 实例中、源文件仍存在且你有权读取源表时，附件
才能重新关联；否则其名称会列入问题报告。关联字段匹配同一工作簿中有权限访问的目标；
未匹配或存在多个匹配的数据会被报告。导入后请检查问题报告和相关依赖。

### 7. 回收站和备份

在保留期限内，所有者可以从**回收站**恢复已删除的工作簿、数据表、视图和字段。
请检查恢复提示，包括可选的字段恢复。

管理员可以打开**备份**，选择**创建备份**，等待状态完成，然后**下载**归档并
妥善保存。恢复备份需要运维人员使用文档中的服务器命令行工具；数据包导出不能
代替备份。运维流程请参阅[部署指南](13-deployment.md)。

管理员可以打开**设置 → 电子邮件（SMTP）**，配置并保存邮件服务器，然后发送测试
邮件。凭据字段留空可保留原值，也可以选择移除控件清除原值。保存 SMTP 密码前，
运维人员需按部署指南配置加密密钥。
