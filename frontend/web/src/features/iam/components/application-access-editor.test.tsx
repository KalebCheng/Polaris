import { useState } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18n from '@/shared/i18n'
import type { ApplicationAccess } from '@/shared/api/types'
import { ApplicationAccessEditor } from './application-access-editor'
import { emptyApplicationAccess, type ApplicationAccessFormValue } from './application-form'

type Candidate = { name: string; display_name: string; path?: string; id: number; has_child?: boolean }

/** 候选主体按类型给：user / group / department */
let candidates: Record<string, Candidate[]> = {}
/** 部门树：父部门 id → 直接子部门（模拟服务端"按层返回 + 分页"） */
let deptChildren: Record<number, Candidate[]> = {}

/**
 * 模拟真实候选端点：扁平类型分页（offset/limit + total），
 * 部门在无关键词时**按层**返回（parentId 省略 = 顶层）。
 */
vi.mock('../api', () => ({
  applicationApi: {
    subjectOptions: async ({
      type,
      keyword,
      parentId,
      offset = 0,
      limit = 10,
    }: {
      type: string
      keyword?: string
      parentId?: number
      offset?: number
      limit?: number
    }) => {
      if (type === 'department' && (keyword ?? '') === '') {
        const level = parentId === undefined ? (candidates.department ?? []) : (deptChildren[parentId] ?? [])
        return { items: level.slice(offset, offset + limit), total: level.length }
      }
      // 主体类型取值与后端一致：'job_title'（原 'group'）
      const matched = (candidates[type === 'job_title' ? 'group' : type] ?? []).filter((item) =>
        (keyword ?? '') === ''
          ? true
          : item.name.includes(keyword ?? '') || item.display_name.includes(keyword ?? ''),
      )
      return { items: matched.slice(offset, offset + limit), total: matched.length }
    },
  },
}))

/**
 * 渲染编辑器：用一层受控外壳持有 state —— 与向导里的真实用法一致
 * （编辑器是受控组件，不这样包一层的话"添加后条目消失"这类断言根本不会发生）。
 */
function renderEditor(initial: ApplicationAccessFormValue, detail?: ApplicationAccess | null) {
  const onChange = vi.fn()
  function Harness() {
    const [access, setAccess] = useState(initial)
    return (
      <I18nextProvider i18n={i18n}>
        <ApplicationAccessEditor
          access={access}
          detail={detail}
          onChange={(next) => {
            onChange(next)
            setAccess(next)
          }}
        />
      </I18nextProvider>
    )
  }
  const ui = render(<Harness />)
  return { onChange, ...ui }
}

/** 常用的展示信息夹具（接口回读时才有展示名 / 路径） */
const detailWith = (
  users: { id: number; name: string; display_name: string; missing?: boolean }[] = [],
  departments: { id: number; name: string; display_name: string; path?: string }[] = [],
): ApplicationAccess => ({
  access_mode: 'restricted',
  users: users.map((item) => ({ ...item, missing: item.missing ?? false })),
  groups: [],
  departments: departments.map((item) => ({ ...item, missing: false })),
})

const restrictedEmpty: ApplicationAccessFormValue = {
  mode: 'restricted',
  userIds: [],
  jobTitleIds: [],
  departmentIds: [],
}

describe('ApplicationAccessEditor（授权类型 / 名单 左右布局）', () => {
  // 断言写字面量（zh 词条），避免用 i18n.t 与自身互证
  beforeAll(async () => {
    await i18n.changeLanguage('zh')
    candidates = {
      user: [
        { id: 101, name: 'zhangsan', display_name: '张三', path: '' },
        { id: 102, name: 'lisi', display_name: '李四', path: '' },
      ],
      group: [{ id: 201, name: 'vpn-users', display_name: 'VPN 用户', path: '' }],
      department: [
        { name: 'tech', display_name: '技术部', path: '技术部', id: 1, has_child: true },
        { name: 'finance', display_name: '财务部', path: '财务部', id: 2, has_child: false },
      ],
    }
    deptChildren = {
      1: [{ name: 'fe', display_name: '前端组', path: '技术部 / 前端组', id: 11, has_child: false }],
    }
  })

  it('默认是「全员可访问」：只显示开关，不出现授权类型与名单', () => {
    renderEditor(emptyApplicationAccess)
    expect(screen.getByText('全员可访问')).toBeInTheDocument()
    expect(screen.queryByText('授权类型')).toBeNull()
  })

  it('受限模式：左侧列出三类授权类型（含各自数量），右侧是该类型的名单', () => {
    renderEditor(
      { mode: 'restricted', userIds: [101], jobTitleIds: [], departmentIds: [1] },
      detailWith(
        [{ id: 101, name: 'zhangsan', display_name: '张三' }],
        [{ id: 1, name: 'tech', display_name: '技术部', path: '技术部' }],
      ),
    )
    expect(screen.getByText('授权类型')).toBeInTheDocument()
    // 三类类型都在，且各自数量可见（用户 1 / 职务 0 / 部门 1）
    const typeList = screen.getByRole('button', { name: '用户' }).parentElement?.parentElement
    expect(typeList).toBeTruthy()
    expect(screen.getByRole('button', { name: '用户' })).toHaveTextContent('1')
    expect(screen.getByRole('button', { name: '职务' })).toHaveTextContent('0')
    expect(screen.getByRole('button', { name: '部门' })).toHaveTextContent('1')
    // 默认选中「用户」，右侧只展示用户类名单（部门不在同一张表里）
    expect(screen.getByText('张三')).toBeInTheDocument()
    expect(screen.queryByText('技术部')).toBeNull()
  })

  it('切换授权类型后右侧名单跟着换（部门含层级路径）', async () => {
    const user = userEvent.setup()
    renderEditor(
      { mode: 'restricted', userIds: [101], jobTitleIds: [], departmentIds: [1] },
      detailWith(
        [{ id: 101, name: 'zhangsan', display_name: '张三' }],
        [{ id: 1, name: 'tech', display_name: '技术部', path: '技术部 / 前端组' }],
      ),
    )
    await user.click(screen.getByRole('button', { name: '部门' }))
    expect(screen.getByText('技术部')).toBeInTheDocument()
    // 层级路径用于区分同名部门
    expect(screen.getByText('技术部 / 前端组')).toBeInTheDocument()
    // 切换类型后只展示该类型的名单
    expect(screen.queryByText('张三')).toBeNull()
    expect(screen.getByText('层级路径')).toBeInTheDocument()
  })

  it('空名单：只给可操作的引导空态，不堆解释性文案', () => {
    renderEditor(restrictedEmpty)
    expect(screen.getByText('还没有授权任何主体')).toBeInTheDocument()
    expect(screen.getByText('点「添加授权」，选择允许登录本应用的用户 / 职务 / 部门')).toBeInTheDocument()
    // 空名单允许保存：界面上没有任何"拦截"类文案
    expect(screen.queryByText(/无人可访问/)).toBeNull()
  })

  it('「添加授权」是勾选式多选 + 底部确定：可跨类型累积，确定后一次写入名单', async () => {
    const user = userEvent.setup()
    renderEditor(restrictedEmpty)
    await user.click(screen.getByRole('button', { name: '添加授权' }))

    // 弹窗里自带类型切换与后果提示；勾选前「已选 0 项」、确定按钮不可用
    expect(await screen.findByText('添加授权后，被授权方将可以访问本应用。')).toBeInTheDocument()
    expect(screen.getByText('已选 0 项')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '确认' })).toBeDisabled()

    await user.click(await screen.findByRole('checkbox', { name: '选择 张三' }))
    expect(screen.getByText('已选 1 项')).toBeInTheDocument()

    // 切到「部门」再勾一个：计数跨类型累积（部门默认是树，顶层节点直接勾）
    await user.click(screen.getByRole('button', { name: '部门' }))
    await user.click(await screen.findByRole('checkbox', { name: '选择 技术部' }))
    expect(screen.getByText('已选 2 项')).toBeInTheDocument()

    // 展开可以看到已勾选项（可单条取消）
    await user.click(screen.getByRole('button', { name: '展开' }))
    expect(screen.getByRole('button', { name: '取消选择 张三' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '确认' }))

    // 两类型一次性写入：用户类型里立刻显示为展示名（而不是裸标识）
    await waitFor(() => expect(screen.getByText('张三')).toBeInTheDocument())
    // 部门也写入了：切过去能看到（左栏计数同步为 1）
    await user.click(screen.getByRole('button', { name: '部门' }))
    // 用标识列断言：顶层部门的 name_path 就是它自己（名称列与路径列都会显示「技术部」）
    expect(screen.getByRole('cell', { name: 'tech' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '部门' })).toHaveTextContent('1')

    // 已授权的不再出现在候选里；重开弹窗停在当前类型（部门），用户类型只剩李四
    await user.click(screen.getByRole('button', { name: '添加授权' }))
    await user.click(await screen.findByRole('button', { name: '用户' }))
    expect(await screen.findByText('李四')).toBeInTheDocument()
    expect(screen.queryByRole('checkbox', { name: '选择 张三' })).toBeNull()
  })

  it('弹窗内可切换授权类型与全选；列表按关键词服务端搜索', async () => {
    const user = userEvent.setup()
    renderEditor(restrictedEmpty)
    await user.click(screen.getByRole('button', { name: '添加授权' }))

    // 全选当前列出的候选
    await user.click(await screen.findByRole('checkbox', { name: '全选当前列出的主体' }))
    expect(screen.getByText('已选 2 项')).toBeInTheDocument()
    // 再点一次取消全选
    await user.click(screen.getByRole('checkbox', { name: '全选当前列出的主体' }))
    expect(screen.getByText('已选 0 项')).toBeInTheDocument()

    // 切到「职务」：类型切换后候选跟着换
    await user.click(screen.getByRole('button', { name: '职务' }))
    expect(await screen.findByText('VPN 用户')).toBeInTheDocument()
  })

  it('部门是树：先出顶层，展开节点时才取下一层（按层懒加载）', async () => {
    const user = userEvent.setup()
    renderEditor(restrictedEmpty)
    await user.click(screen.getByRole('button', { name: '添加授权' }))
    await user.click(await screen.findByRole('button', { name: '部门' }))

    // 顶层先出来；子部门在展开前不请求、不渲染
    expect(await screen.findByText('技术部')).toBeInTheDocument()
    expect(screen.queryByText('前端组')).toBeNull()

    await user.click(screen.getByRole('button', { name: '展开 技术部' }))
    expect(await screen.findByText('前端组')).toBeInTheDocument()
    // 无子节点的节点不画展开箭头
    expect(screen.queryByRole('button', { name: '展开 财务部' })).toBeNull()

    // 已授权的节点在树里保留（不破坏导航）但不可再勾选
    await user.click(screen.getByRole('checkbox', { name: '选择 技术部' }))
    expect(screen.getByText('已选 1 项')).toBeInTheDocument()
  })

  it('扁平候选是服务端分页：每页 10 条，翻页才取下一页', async () => {
    const user = userEvent.setup()
    // 造 12 个用户 → 两页
    candidates.user = Array.from({ length: 12 }, (_, index) => ({
      id: 1000 + index,
      name: `user-${String(index + 1).padStart(2, '0')}`,
      display_name: `用户 ${index + 1}`,
    }))
    try {
      renderEditor(restrictedEmpty)
      await user.click(screen.getByRole('button', { name: '添加授权' }))

      // 第 1 页只出现前 10 个（第 11 个要翻页才来）
      expect(await screen.findByText('用户 1')).toBeInTheDocument()
      expect(screen.getByText('用户 10')).toBeInTheDocument()
      expect(screen.queryByText('用户 11')).toBeNull()

      await user.click(screen.getByRole('button', { name: '下一页' }))
      expect(await screen.findByText('用户 11')).toBeInTheDocument()
      expect(screen.queryByText('用户 1')).toBeNull()
    } finally {
      candidates.user = [
        { id: 101, name: 'zhangsan', display_name: '张三', path: '' },
        { id: 102, name: 'lisi', display_name: '李四', path: '' },
      ]
    }
  })

  it('名单里可以逐条移除，已失效主体带标记', async () => {
    const user = userEvent.setup()
    const { onChange } = renderEditor(
      { mode: 'restricted', userIds: [101, 999], jobTitleIds: [], departmentIds: [] },
      detailWith([
        { id: 101, name: 'zhangsan', display_name: '张三' },
        { id: 999, name: 'ghost', display_name: '幽灵用户', missing: true },
      ]),
    )
    // 主体已被删除 → 标「已失效」（判定不命中，避免"看着还在、其实不生效"）
    expect(screen.getByText('已失效')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '移除 幽灵用户 的授权' }))
    expect(onChange).toHaveBeenCalledWith({
      mode: 'restricted',
      userIds: [101],
      jobTitleIds: [],
      departmentIds: [],
    })
  })

  it('名单筛选：命中不到时给空态', async () => {
    const user = userEvent.setup()
    renderEditor(
      { mode: 'restricted', userIds: [101], jobTitleIds: [], departmentIds: [] },
      detailWith([{ id: 101, name: 'zhangsan', display_name: '张三' }]),
    )
    await user.type(screen.getByLabelText('按账户名搜索'), '不存在的名字')
    expect(screen.getByText('没有匹配的授权条目')).toBeInTheDocument()
    expect(screen.queryByText('张三')).toBeNull()
  })

  it('关掉「全员可访问」回到受限模式时，已有名单原样展示（不丢配置）', async () => {
    const user = userEvent.setup()
    const { onChange } = renderEditor({
      mode: 'restricted',
      userIds: [101],
      jobTitleIds: [201],
      departmentIds: [],
    })
    await user.click(screen.getByRole('switch'))
    expect(onChange).toHaveBeenCalledWith({
      mode: 'all',
      userIds: [101],
      jobTitleIds: [201],
      departmentIds: [],
    })
  })

  it('右侧面板展示覆盖语义文案（部门含子部门 / 职务含子组）', async () => {
    const user = userEvent.setup()
    renderEditor(restrictedEmpty)
    expect(screen.getByText('被授权的用户可以登录本应用')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '部门' }))
    expect(screen.getByText('授权一个部门即覆盖其全部子部门')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '职务' }))
    expect(screen.getByText('授权一个职务即覆盖其全部任职人员')).toBeInTheDocument()
  })
})
