import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { Button, Combobox, Input, Skeleton, Textarea } from '@/shared/ui'
import { PageShell } from '@/widgets/layout/page-shell'
import { FormSection } from '@/shared/components/form-section'
import { SwitchField } from '@/shared/components/switch-field'
import { ConfirmDialog } from '@/shared/components/confirm-dialog'
import type { Department, User, UserProfileInput } from '@/shared/api/types'
import { departmentApi, userApi } from '../api'
import { useWorkspaceInstitution, useWorkspaceInstitutionId } from '../institution-context'
import { useMutation } from '../hooks/use-mutation'
import { useOnceKey } from '../hooks/use-once-key'
import { Field } from '../components/field'

/** 表单状态：新建与编辑共用同一套字段 */
interface UserForm {
  name: string
  display_name: string
  first_name: string
  last_name: string
  avatar: string
  email: string
  email_verified: boolean
  phone: string
  phone_verified: boolean
  country_code: string
  location: string
  address: string
  affiliation: string
  title: string
  homepage: string
  bio: string
  language: string
  gender: string
  birthday: string
  education: string
  department_id: string
  is_admin: boolean
  password: string
}

const emptyForm: UserForm = {
  name: '',
  display_name: '',
  first_name: '',
  last_name: '',
  avatar: '',
  email: '',
  email_verified: false,
  phone: '',
  phone_verified: false,
  country_code: '',
  location: '',
  address: '',
  affiliation: '',
  title: '',
  homepage: '',
  bio: '',
  language: '',
  gender: '',
  birthday: '',
  education: '',
  department_id: '',
  is_admin: false,
  password: '',
}

function toForm(user: User): UserForm {
  return {
    name: user.name,
    display_name: user.display_name ?? '',
    first_name: user.first_name ?? '',
    last_name: user.last_name ?? '',
    avatar: user.avatar ?? '',
    email: user.email ?? '',
    email_verified: user.email_verified,
    phone: user.phone ?? '',
    phone_verified: user.phone_verified,
    country_code: user.country_code ?? '',
    location: user.location ?? '',
    address: user.address ?? '',
    affiliation: user.affiliation ?? '',
    title: user.title ?? '',
    homepage: user.homepage ?? '',
    bio: user.bio ?? '',
    language: user.language ?? '',
    gender: user.gender ?? '',
    birthday: user.birthday ?? '',
    education: user.education ?? '',
    department_id: user.department_id ? String(user.department_id) : '',
    is_admin: user.is_admin,
    password: '',
  }
}

/** 逐字段比对：编辑时只提交**真正变化**的字段（与后端"只覆盖提交字段"合成部分更新契约） */
function buildProfilePatch(baseline: UserForm, form: UserForm): UserProfileInput {
  const keys: (keyof UserForm)[] = [
    'display_name',
    'first_name',
    'last_name',
    'avatar',
    'email',
    'email_verified',
    'phone',
    'phone_verified',
    'country_code',
    'location',
    'address',
    'affiliation',
    'title',
    'homepage',
    'bio',
    'language',
    'gender',
    'birthday',
    'education',
  ]
  const patch: Record<string, string | boolean> = {}
  for (const key of keys) {
    if (form[key] !== baseline[key]) patch[key] = form[key] as string | boolean
  }
  return patch as UserProfileInput
}

/**
 * 用户新建 / 编辑（`/institutions/:inst/users/new` 与 `/users/:id/edit`）。
 *
 * 为什么是**单页多列分区表单**而不是分步向导：用户的 20 个字段彼此无关、也没有先后依赖，
 * 切成四步只会让"一步两三个字段"在宽屏上铺不满、显得像玩具。这里按语义分区、每区多列并排，
 * 一屏看完、一行铺满——改一个字段不必先点三次「下一步」。
 * （向导留给"阶段之间确实有依赖"的流程，如应用五步：登录方式决定后续选项。）
 */
export default function UserEditPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const institution = useWorkspaceInstitution()
  const institutionId = useWorkspaceInstitutionId()
  const { pending, run } = useMutation()

  const isEdit = Boolean(id)
  const [form, setForm] = useState<UserForm>(emptyForm)
  const [baseline, setBaseline] = useState<UserForm>(emptyForm)
  const [departments, setDepartments] = useState<Department[]>([])
  const [loading, setLoading] = useState(true)
  const [discardOpen, setDiscardOpen] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [deptPage, user] = await Promise.all([
        departmentApi.list({ institution, page: 1, size: 200 }).catch(() => ({ items: [] as Department[] })),
        id ? userApi.get(Number(id), institution || undefined) : Promise.resolve(null),
      ])
      setDepartments(deptPage.items ?? [])
      if (user) {
        const nextForm = toForm(user)
        setForm(nextForm)
        setBaseline(nextForm)
      }
    } finally {
      setLoading(false)
    }
  }, [id, institution])

  // 引导请求按 key 守卫：StrictMode 下只发一次
  useOnceKey(`user-edit:${id ?? 'new'}:${institution}`, load)

  const update = <K extends keyof UserForm>(key: K, value: UserForm[K]) =>
    setForm((prev) => ({ ...prev, [key]: value }))

  const patch = buildProfilePatch(baseline, form)
  const dirty = isEdit
    ? Object.keys(patch).length > 0 ||
      form.department_id !== baseline.department_id ||
      form.is_admin !== baseline.is_admin
    : Object.values(form).some((value) => value !== '' && value !== false)

  const backTo = () =>
    navigate(id ? `/institutions/${institutionId}/users/${id}` : `/institutions/${institutionId}/users`)

  const submit = async () => {
    if (!institution) return
    const departmentId = form.department_id ? Number(form.department_id) : null
    await run(async () => {
      if (isEdit && id) {
        if (Object.keys(patch).length > 0) await userApi.updateProfile(Number(id), patch)
        if (form.department_id !== baseline.department_id) await userApi.setDepartment(Number(id), departmentId)
        if (form.is_admin !== baseline.is_admin) await userApi.setAdmin(Number(id), form.is_admin)
        navigate(`/institutions/${institutionId}/users/${id}`)
        return
      }
      const created = await userApi.create({
        name: form.name,
        display_name: form.display_name,
        first_name: form.first_name,
        last_name: form.last_name,
        avatar: form.avatar,
        email: form.email,
        email_verified: form.email_verified,
        phone: form.phone,
        phone_verified: form.phone_verified,
        country_code: form.country_code,
        location: form.location,
        address: form.address,
        affiliation: form.affiliation,
        title: form.title,
        homepage: form.homepage,
        bio: form.bio,
        language: form.language,
        gender: form.gender,
        birthday: form.birthday,
        education: form.education,
        department_id: departmentId,
        is_admin: form.is_admin,
        password: form.password || undefined,
      })
      navigate(`/institutions/${institutionId}/users/${created.id}`)
    }, isEdit ? 'users.savedDone' : 'users.createdDone')
  }

  const departmentOptions = departments.map((dept) => ({
    value: String(dept.id),
    label: dept.name_path || dept.name,
  }))

  return (
    <PageShell
      title={isEdit ? t('users.editTitle') : t('users.newTitle')}
      description={isEdit ? form.name : t('users.newHint')}
      actions={
        <div className="flex items-center gap-2">
          <Button variant="ghost" onClick={() => (dirty ? setDiscardOpen(true) : backTo())} disabled={pending}>
            {t('common.cancel')}
          </Button>
          <Button onClick={() => void submit()} disabled={pending || (!isEdit && form.name.trim() === '')}>
            {isEdit ? t('common.save') : t('common.create')}
          </Button>
        </div>
      }
    >
      {loading ? (
        <div className="space-y-4 pt-1">
          <Skeleton className="h-40 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      ) : (
        <div className="space-y-4 pt-1 pb-2">
          {/* 分区按语义自上而下；每个分区内部多列并排，一行铺满内容区宽度 */}
          <FormSection titleKey="users.sectionBasic" descriptionKey="users.sectionBasicHint">
            <div className="grid gap-x-6 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
              <Field
                id="user-name"
                labelKey="users.name"
                hint={isEdit ? t('users.nameImmutableHint') : t('users.nameHint')}
              >
                <Input
                  id="user-name"
                  value={form.name}
                  disabled={isEdit}
                  onChange={(event) => update('name', event.target.value)}
                />
              </Field>
              <Field id="user-display-name" labelKey="users.displayName">
                <Input
                  id="user-display-name"
                  value={form.display_name}
                  onChange={(event) => update('display_name', event.target.value)}
                />
              </Field>
              <Field id="user-last-name" labelKey="users.lastName">
                <Input
                  id="user-last-name"
                  value={form.last_name}
                  onChange={(event) => update('last_name', event.target.value)}
                />
              </Field>
              <Field id="user-first-name" labelKey="users.firstName">
                <Input
                  id="user-first-name"
                  value={form.first_name}
                  onChange={(event) => update('first_name', event.target.value)}
                />
              </Field>
              <Field id="user-language" labelKey="users.language">
                <Input
                  id="user-language"
                  value={form.language}
                  onChange={(event) => update('language', event.target.value)}
                />
              </Field>
              <Field id="user-title-field" labelKey="users.titleField">
                <Input
                  id="user-title-field"
                  value={form.title}
                  onChange={(event) => update('title', event.target.value)}
                />
              </Field>
              <Field
                id="user-avatar"
                labelKey="users.avatar"
                hint={t('users.avatarHint')}
                className="space-y-2 xl:col-span-2"
              >
                <Input id="user-avatar" value={form.avatar} onChange={(event) => update('avatar', event.target.value)} />
              </Field>
            </div>
          </FormSection>

          <FormSection titleKey="users.sectionContact" descriptionKey="users.sectionContactHint">
            <div className="grid gap-x-6 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
              <Field id="user-email" labelKey="users.email">
                <Input
                  id="user-email"
                  type="email"
                  value={form.email}
                  onChange={(event) => update('email', event.target.value)}
                />
              </Field>
              <Field id="user-phone" labelKey="users.phone">
                <Input id="user-phone" value={form.phone} onChange={(event) => update('phone', event.target.value)} />
              </Field>
              <Field id="user-country-code" labelKey="users.countryCode">
                <Input
                  id="user-country-code"
                  value={form.country_code}
                  onChange={(event) => update('country_code', event.target.value)}
                />
              </Field>
              <div className="space-y-3 self-end">
                <SwitchField
                  labelKey="users.emailVerified"
                  checked={form.email_verified}
                  onCheckedChange={(checked) => update('email_verified', checked)}
                />
                <SwitchField
                  labelKey="users.phoneVerified"
                  checked={form.phone_verified}
                  onCheckedChange={(checked) => update('phone_verified', checked)}
                />
              </div>
            </div>
          </FormSection>

          <FormSection titleKey="users.sectionMembership" descriptionKey="users.sectionMembershipHint">
            <div className="grid gap-x-6 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
              <Field id="user-department" labelKey="users.department" hint={t('users.departmentHint')}>
                <Combobox
                  options={departmentOptions}
                  value={form.department_id}
                  onChange={(value) => update('department_id', value)}
                  placeholder={t('users.departmentPlaceholder')}
                />
              </Field>
              {!isEdit ? (
                <Field id="user-password" labelKey="users.password" hint={t('users.passwordHint')}>
                  <Input
                    id="user-password"
                    type="password"
                    value={form.password}
                    onChange={(event) => update('password', event.target.value)}
                    placeholder={t('users.passwordPlaceholder')}
                  />
                </Field>
              ) : null}
              <div className={isEdit ? 'space-y-3 sm:col-span-2 xl:col-span-3' : 'space-y-3 sm:col-span-2 xl:col-span-2'}>
                <SwitchField
                  labelKey="users.isAdmin"
                  descriptionKey="users.isAdminHint"
                  checked={form.is_admin}
                  onCheckedChange={(checked) => update('is_admin', checked)}
                />
              </div>
              <p className="text-muted-foreground text-xs sm:col-span-2 xl:col-span-4">
                {isEdit ? t('users.asidePasswordNote') : t('users.passwordPolicyHint')}
              </p>
            </div>
          </FormSection>

          <FormSection titleKey="users.sectionProfileOther" descriptionKey="users.sectionProfileOtherHint">
            <div className="grid gap-x-6 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
              <Field id="user-affiliation" labelKey="users.affiliation">
                <Input
                  id="user-affiliation"
                  value={form.affiliation}
                  onChange={(event) => update('affiliation', event.target.value)}
                />
              </Field>
              <Field id="user-location" labelKey="users.location">
                <Input
                  id="user-location"
                  value={form.location}
                  onChange={(event) => update('location', event.target.value)}
                />
              </Field>
              <Field id="user-address" labelKey="users.address">
                <Input
                  id="user-address"
                  value={form.address}
                  onChange={(event) => update('address', event.target.value)}
                />
              </Field>
              <Field id="user-homepage" labelKey="users.homepage">
                <Input
                  id="user-homepage"
                  value={form.homepage}
                  onChange={(event) => update('homepage', event.target.value)}
                />
              </Field>
              <Field id="user-gender" labelKey="users.gender">
                <Input id="user-gender" value={form.gender} onChange={(event) => update('gender', event.target.value)} />
              </Field>
              <Field id="user-birthday" labelKey="users.birthday">
                <Input
                  id="user-birthday"
                  value={form.birthday}
                  onChange={(event) => update('birthday', event.target.value)}
                />
              </Field>
              <Field id="user-education" labelKey="users.education">
                <Input
                  id="user-education"
                  value={form.education}
                  onChange={(event) => update('education', event.target.value)}
                />
              </Field>
              <Field id="user-bio" labelKey="users.bio" className="space-y-2 sm:col-span-2 xl:col-span-4">
                <Textarea
                  id="user-bio"
                  rows={2}
                  value={form.bio}
                  onChange={(event) => update('bio', event.target.value)}
                />
              </Field>
            </div>
          </FormSection>

          <div className="flex items-center justify-between gap-3 pt-1">
            <Button variant="ghost" onClick={backTo} disabled={pending}>
              <ArrowLeft className="size-4" /> {isEdit ? t('users.backToDetail') : t('users.backToList')}
            </Button>
            <Button onClick={() => void submit()} disabled={pending || (!isEdit && form.name.trim() === '')}>
              {isEdit ? t('common.save') : t('common.create')}
            </Button>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={discardOpen}
        onOpenChange={setDiscardOpen}
        titleKey="applications.unsavedTitle"
        descriptionKey="applications.unsavedBody"
        confirmLabelKey="common.discard"
        onConfirm={() => {
          setDiscardOpen(false)
          backTo()
        }}
      />
    </PageShell>
  )
}
