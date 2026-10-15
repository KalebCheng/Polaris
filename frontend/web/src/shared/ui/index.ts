/**
 * UI 组件统一出口。
 * 红线：页面与 feature 代码只允许从 `@/shared/ui` 导入 UI 组件，
 * 禁止直接引用第三方组件库或裸 HTML 控件组合。
 */
export { Button, buttonVariants, type ButtonProps } from './components/button'
export { Input } from './components/input'
export { InputOTP, InputOTPGroup, InputOTPSlot, InputOTPSeparator } from './components/input-otp'
export { Label } from './components/label'
export { Textarea } from './components/textarea'
export {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardAction,
  CardContent,
  CardFooter,
} from './components/card'
export { Badge, badgeVariants } from './components/badge'
export { Separator } from './components/separator'
export { Skeleton } from './components/skeleton'
export {
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableRow,
  TableHead,
  TableCell,
  TableCaption,
} from './components/table'
export { EmptyState } from './components/empty-state'
export { Spinner } from './components/spinner'
export {
  Dialog,
  DialogTrigger,
  DialogClose,
  DialogPortal,
  DialogOverlay,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from './components/dialog'
export {
  Select,
  SelectGroup,
  SelectValue,
  SelectTrigger,
  SelectContent,
  SelectLabel,
  SelectItem,
  SelectSeparator,
} from './components/select'
export { Switch } from './components/switch'
export { Checkbox } from './components/checkbox'
export { Tabs, TabsList, TabsTrigger, TabsContent } from './components/tabs'
export { SlidingPill, useSlidingPill, type SlidingPillBox } from './components/sliding-pill'
export {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuGroup,
  DropdownMenuPortal,
  DropdownMenuSub,
  DropdownMenuRadioGroup,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuCheckboxItem,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from './components/dropdown-menu'
export { Avatar, AvatarImage, AvatarFallback } from './components/avatar'
export { Toaster } from './components/sonner'
export { TooltipProvider, Tooltip, TooltipTrigger, TooltipContent } from './components/tooltip'
export { ScrollArea, ScrollBar } from './components/scroll-area'
export { MultiSelect, Combobox, type SelectOption } from './components/multi-select'
