import { useTranslation } from 'react-i18next'
import { Badge, Button } from '@/shared/ui'
import { ConnectionTile, type ConnectionIconSpec } from '@/shared/components/connection-icon'
import { StatusPill, type StatusTone } from '@/shared/components/status-pill'

/**
 * 身份连接实例卡片（对齐参考产品的实例卡片）：
 * 标题行（图标 + 名称 + 标签 + 状态与操作）+ 元信息行 + 双列主体（能力开关说明 + 计划/范围与行内操作）。
 */
export interface ConnectionCardLink {
  label: string
  onClick: () => void
}

export interface ConnectionCardSection {
  label: string
  hint: string
  links?: ConnectionCardLink[]
}

export interface ConnectionCardView {
  name: string
  subtitle: string
  icon: ConnectionIconSpec
  type: string
  subType?: string
  tags: string[]
  meta: string[]
  status: { tone: StatusTone; label: string }
  sections: ConnectionCardSection[]
  onEdit: () => void
  /** 省略时不渲染删除入口（例如"尚未配置"的单例：没有东西可清除） */
  onDelete?: () => void
  /** 省略时不渲染启用/停用入口（入方向连接有启用态，出方向由 state 字段承担） */
  onToggle?: () => void
  /** 启用/停用的词条 key（调用方按当前状态给） */
  toggleLabelKey?: string
  /**
   * 主操作 / 删除操作的文案 key。
   * 默认按「身份连接」的措辞（编辑 / 删除）；复用同一张卡片的其它资源（如系统存储设置）
   * 用「修改配置 / 清除配置」这类更贴合自身语义的措辞，不必另写一张卡片。
   */
  editLabelKey?: string
  removeLabelKey?: string
  /**
   * 主操作的按钮形态。默认 `outline`（卡片里已有状态与元信息，按钮只是行内动作）；
   * 传 `default` 用于"还没有配置、配置本身就是页面唯一主操作"的场景（如系统设置 › 存储设置）；
   * 其余场景统一 `ghost`（与卡片内的其它操作同一外观）。
   */
  editVariant?: 'default' | 'ghost'
}

export function ConnectionCard({ view }: { view: ConnectionCardView }) {
  const { t } = useTranslation()
  return (
    <div className="surface overflow-hidden rounded-2xl">
      <div className="flex items-start gap-3 p-4">
        <ConnectionTile icon={view.icon} type={view.type} subType={view.subType} tileSize={40} />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{view.name}</p>
          <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
            {view.tags.map((tag) => (
              <Badge key={tag} variant="secondary" className="text-[11px]">
                {tag}
              </Badge>
            ))}
          </div>
          <p className="text-muted-foreground mt-2 truncate text-[11px]">{view.meta.join(' · ')}</p>
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
          <StatusPill tone={view.status.tone}>{view.status.label}</StatusPill>
          {/*
            卡片操作统一样式：全部用 ghost（无边框文字按钮 + hover 浅背景）。
            此前「修改」是 outline（白底描边+阴影）、「停用/删除」是 ghost，
            并排看不像一组；危险操作靠红色表达语义，而不是靠多一种按钮外观。
          */}
          {view.onToggle ? (
            <Button type="button" size="sm" variant="ghost" onClick={view.onToggle}>
              {t(view.toggleLabelKey ?? 'connections.enable')}
            </Button>
          ) : null}
          <Button
            type="button"
            size="sm"
            variant={view.editVariant === 'default' ? 'default' : 'ghost'}
            onClick={view.onEdit}
          >
            {t(view.editLabelKey ?? 'connections.edit')}
          </Button>
          {view.onDelete ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="text-destructive hover:bg-destructive/10 hover:text-destructive"
              onClick={view.onDelete}
            >
              {t(view.removeLabelKey ?? 'connections.remove')}
            </Button>
          ) : null}
        </div>
      </div>
      <div className="grid grid-cols-1 border-t sm:grid-cols-2">
        {view.sections.map((section, index) => (
          <div key={section.label} className={index > 0 ? 'border-t p-4 sm:border-t-0 sm:border-l' : 'p-4'}>
            <p className="text-[13px] font-semibold">{section.label}</p>
            <p className="text-muted-foreground mt-1.5 text-[11px] leading-relaxed">{section.hint}</p>
            {section.links?.length ? (
              <div className="mt-2 flex flex-wrap items-center gap-3">
                {section.links.map((link) => (
                  // 走 Button 的 link variant（颜色/下划线/hover 由组件统一），
                  // 尺寸按卡片密度微调；不再手写 text-primary hover:underline
                  <Button
                    key={link.label}
                    type="button"
                    variant="link"
                    size="sm"
                    className="h-auto p-0 text-[11px]"
                    onClick={link.onClick}
                  >
                    {link.label}
                  </Button>
                ))}
              </div>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  )
}
