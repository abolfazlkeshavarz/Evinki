package auth

import "strings"

// Permissions are plain strings checked by middleware. Roles are bundles of
// them, stored in role_permissions so an admin can adjust a bundle without a
// deploy; DefaultRoles seeds the first version.
const (
	PCatalogManage = "catalog.manage"
	PCatalogDelete = "catalog.delete"

	PShopRead   = "shop.read"
	PShopManage = "shop.manage"

	PWarrantyRead    = "warranty.read"
	PWarrantyWrite   = "warranty.write"
	PWarrantyApprove = "warranty.approve"
	PWarrantyDelete  = "warranty.delete"

	PServiceRead    = "service.read"
	PServiceManage  = "service.manage"
	PServiceDelete  = "service.delete"
	PTechniciansMgt = "service.technicians"

	PWhRead    = "wh.read"
	PWhRequest = "wh.request" // may file movements; outgoing ones wait for approval
	PWhWrite   = "wh.write"   // movements apply immediately
	PWhApprove = "wh.approve"
	PWhManage  = "wh.manage" // items, counts, exports
	PWhDelete  = "wh.delete"

	PHrRead    = "hr.read"
	PHrManage  = "hr.manage"
	PHrDevices = "hr.devices"
	PHrDelete  = "hr.delete"

	PSurveyRead   = "surveys.read"
	PSurveyManage = "surveys.manage"

	PReports  = "reports.read"
	PAudit    = "audit.read"
	PUsers    = "users.manage"
	PRoles    = "roles.manage"
	PSettings = "settings.manage"

	// Portal permissions: what the apps check to decide which screens to show.
	PTechPortal     = "tech.portal"
	PCustomerPortal = "customer.portal"
)

var AllPermissions = []string{
	PCatalogManage, PCatalogDelete, PShopRead, PShopManage,
	PWarrantyRead, PWarrantyWrite, PWarrantyApprove, PWarrantyDelete,
	PServiceRead, PServiceManage, PServiceDelete, PTechniciansMgt,
	PWhRead, PWhRequest, PWhWrite, PWhApprove, PWhManage, PWhDelete,
	PHrRead, PHrManage, PHrDevices, PHrDelete,
	PSurveyRead, PSurveyManage,
	PReports, PAudit, PUsers, PRoles, PSettings,
	PTechPortal, PCustomerPortal,
}

type RoleDef struct {
	Code, TitleFA, TitleEN string
	Perms                  []string
}

func staffPerms(exceptDelete bool) []string {
	var out []string
	for _, p := range AllPermissions {
		if p == PTechPortal || p == PCustomerPortal {
			continue
		}
		if exceptDelete && (strings.HasSuffix(p, ".delete") || p == PUsers || p == PRoles || p == PSettings) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func readOnlyPerms() []string {
	return []string{PShopRead, PWarrantyRead, PServiceRead, PWhRead, PHrRead, PSurveyRead, PReports}
}

// DefaultRoles mirrors the roles of the three old systems, merged:
//   guarantee: admin, technical (no delete), technician
//   warehouse: admin (+super), storekeeper
//   attendance: admin, manager (سرپرست), viewer (ناظر)
func DefaultRoles() []RoleDef {
	return []RoleDef{
		{"super_admin", "مدیر ارشد", "Super admin", []string{"*"}},
		{"admin", "مدیر سیستم", "Administrator", staffPerms(false)},
		{"technical", "کاربر فنی", "Technical staff (no delete)", staffPerms(true)},
		{"shop_manager", "مدیر فروشگاه", "Shop manager",
			[]string{PCatalogManage, PShopRead, PShopManage, PWarrantyRead, PSurveyRead, PReports}},
		{"warehouse_manager", "مدیر انبار", "Warehouse manager",
			[]string{PWhRead, PWhWrite, PWhApprove, PWhManage, PServiceRead, PReports}},
		{"storekeeper", "انباردار", "Storekeeper", []string{PWhRead, PWhRequest}},
		{"hr_manager", "سرپرست منابع انسانی", "HR supervisor",
			[]string{PHrRead, PHrManage, PHrDevices, PReports}},
		{"viewer", "ناظر", "Viewer", readOnlyPerms()},
		{"technician", "تکنسین", "Technician", []string{PTechPortal}},
		{"customer", "مشتری", "Customer", []string{PCustomerPortal}},
	}
}

var PermissionTitles = map[string]string{
	PCatalogManage: "مدیریت کاتالوگ محصولات و قطعات", PCatalogDelete: "حذف از کاتالوگ",
	PShopRead: "مشاهده سفارش‌ها", PShopManage: "مدیریت سفارش‌ها و تخفیف‌ها",
	PWarrantyRead: "مشاهده گارانتی‌ها", PWarrantyWrite: "ثبت و ویرایش گارانتی",
	PWarrantyApprove: "تایید/رد/تمدید گارانتی", PWarrantyDelete: "حذف گارانتی",
	PServiceRead: "مشاهده خدمات پس از فروش", PServiceManage: "مدیریت تعمیرات، درخواست قطعه، ارسال و صورتحساب",
	PServiceDelete: "حذف در خدمات پس از فروش", PTechniciansMgt: "مدیریت تکنسین‌ها",
	PWhRead: "مشاهده انبار", PWhRequest: "ثبت حرکت انبار (خروج با تایید)",
	PWhWrite: "ثبت حرکت انبار (بدون تایید)", PWhApprove: "تایید حرکت‌های انبار",
	PWhManage: "مدیریت اقلام، انبارگردانی و خروجی‌ها", PWhDelete: "حذف سند انبار",
	PHrRead: "مشاهده حضور و غیاب", PHrManage: "مدیریت پرسنل، مرخصی و وظایف",
	PHrDevices: "مدیریت دستگاه‌های حضور و غیاب", PHrDelete: "حذف در منابع انسانی",
	PSurveyRead: "مشاهده نظرسنجی‌ها", PSurveyManage: "مدیریت نظرسنجی‌ها",
	PReports: "گزارش‌ها و داشبورد", PAudit: "گزارش ممیزی", PUsers: "مدیریت کاربران",
	PRoles: "مدیریت نقش‌ها", PSettings: "تنظیمات سامانه",
	PTechPortal: "پنل تکنسین", PCustomerPortal: "پنل مشتری",
}
