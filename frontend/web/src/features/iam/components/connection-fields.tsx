import { SchemaFields, type FieldValueMap } from '@/shared/components/schema-fields'
import type { ConnectionField } from '@/shared/api/types'

/** 表单值（转发自共享的 schema 渲染器） */
export type { FieldValueMap }

interface ConnectionFieldsProps {
  fields: ConnectionField[]
  /** 当前类型 key：类型专属词条优先（如 connections.field.ldap.user） */
  typeKey?: string
  values: FieldValueMap
  onChange: (key: string, value: string | boolean | string[]) => void
  /** 编辑态：敏感字段已配置时提示"留空表示不修改" */
  editing?: boolean
  /** 组织下拉的可选项（FieldDepartment 用） */
  departments?: { id: number; name: string; display_name: string }[]
  applications?: { id: number; code: string; name: string }[]
}

/**
 * 身份连接的字段渲染器 = 共享 schema 渲染器 + connections 词条前缀。
 * 提供商（providers）用同一份实现，避免两套渲染器各自漂移。
 */
export function ConnectionFields({
  fields,
  typeKey,
  values,
  onChange,
  editing,
  departments,
  applications,
}: ConnectionFieldsProps) {
  return (
    <SchemaFields
      fields={fields}
      typeKey={typeKey}
      values={values}
      onChange={onChange}
      i18nPrefix="connections"
      editing={editing}
      departments={departments}
      dynamicOptions={{
        application_id: (applications ?? []).map((app) => ({
          value: String(app.id),
          label: app.name ? `${app.name}（${app.code}）` : app.code,
        })),
      }}
    />
  )
}
