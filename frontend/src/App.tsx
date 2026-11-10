import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { AccountLayout, AdminLayout, TechLayout } from './layouts'
import { ShopLayout } from './pages/shop/ShopLayout'
import { RequireAuth } from './lib/auth'
import { Loading } from './components/ui'

// Each area is a separate chunk: customers never download the back-office code.
const L = <T extends Record<string, any>>(f: () => Promise<T>, name: keyof T) => lazy(() => f().then((m) => ({ default: m[name] })))

const Home = L(() => import('./pages/shop/Shop'), 'Home')
const Products = L(() => import('./pages/shop/Shop'), 'Products')
const Accessories = L(() => import('./pages/shop/Shop'), 'Accessories')
const ProductPage = L(() => import('./pages/shop/Shop'), 'ProductPage')
const Cart = L(() => import('./pages/shop/Cart'), 'Cart')
const Checkout = L(() => import('./pages/shop/Cart'), 'Checkout')
const GuaranteePublic = L(() => import('./pages/shop/GuaranteePublic'), 'GuaranteePublic')
const Login = L(() => import('./pages/Login'), 'Login')
const TechApply = L(() => import('./pages/Login'), 'TechApply')
const SurveyPublic = L(() => import('./pages/Login'), 'SurveyPublic')

const AccountHome = L(() => import('./pages/account/Account'), 'AccountHome')
const MyOrders = L(() => import('./pages/account/Account'), 'MyOrders')
const MyOrder = L(() => import('./pages/account/Account'), 'MyOrder')
const MyGuarantees = L(() => import('./pages/account/Account'), 'MyGuarantees')
const Addresses = L(() => import('./pages/account/Account'), 'Addresses')
const Wishlist = L(() => import('./pages/account/Account'), 'Wishlist')
const Profile = L(() => import('./pages/account/Account'), 'Profile')

const Overview = L(() => import('./pages/admin/Overview'), 'Overview')
const Orders = L(() => import('./pages/admin/Orders'), 'Orders')
const OrderDetail = L(() => import('./pages/admin/Orders'), 'OrderDetail')
const Products_ = L(() => import('./pages/admin/Catalog'), 'AdminProducts')
const Categories = L(() => import('./pages/admin/Catalog'), 'Categories')
const Parts = L(() => import('./pages/admin/Catalog'), 'Parts')
const Coupons = L(() => import('./pages/admin/Catalog'), 'Coupons')
const Guarantees = L(() => import('./pages/admin/Guarantees'), 'Guarantees')
const GuaranteeDetail = L(() => import('./pages/admin/Guarantees'), 'GuaranteeDetail')
const Customers = L(() => import('./pages/admin/Guarantees'), 'Customers')
const ServicePipeline = L(() => import('./pages/admin/Service'), 'Pipeline')
const RepairDetail = L(() => import('./pages/admin/Service'), 'RepairDetail')
const PartRequests = L(() => import('./pages/admin/Service'), 'PartRequests')
const Shipments = L(() => import('./pages/admin/Service'), 'Shipments')
const ShipmentDetail = L(() => import('./pages/admin/Service'), 'ShipmentDetail')
const Technicians = L(() => import('./pages/admin/Service'), 'Technicians')
const TechnicianDetail = L(() => import('./pages/admin/Service'), 'TechnicianDetail')
const Stock = L(() => import('./pages/admin/Warehouse'), 'Stock')
const Scan = L(() => import('./pages/admin/Warehouse'), 'Scan')
const Documents = L(() => import('./pages/admin/Warehouse'), 'Documents')
const Counts = L(() => import('./pages/admin/Warehouse'), 'Counts')
const Labels = L(() => import('./pages/admin/Warehouse'), 'Labels')
const Employees = L(() => import('./pages/admin/HrEmployees'), 'Employees')
const HrDashboard = L(() => import('./pages/admin/HrDash'), 'HrDashboard')
const Attendance = L(() => import('./pages/admin/HrDash'), 'AttendancePage')
const Kiosk = lazy(() => import('./pages/kiosk/Kiosk'))
const LeavePublic = lazy(() => import('./pages/kiosk/LeavePublic'))
const HrReport = L(() => import('./pages/admin/HrDash'), 'Report')
const Leaves = L(() => import('./pages/admin/Hr'), 'Leaves')
const Tasks = L(() => import('./pages/admin/Hr'), 'Tasks')
const Devices = L(() => import('./pages/admin/Hr'), 'Devices')
const Shifts = L(() => import('./pages/admin/Hr'), 'Shifts')
const Surveys = L(() => import('./pages/admin/Surveys'), 'Surveys')
const SurveyEditor = L(() => import('./pages/admin/Surveys'), 'SurveyEditor')
const SurveyResults = L(() => import('./pages/admin/Surveys'), 'SurveyResults')
const Users = L(() => import('./pages/admin/System'), 'Users')
const Roles = L(() => import('./pages/admin/System'), 'Roles')
const Sms = L(() => import('./pages/admin/System'), 'Sms')
const SettingsPage = L(() => import('./pages/admin/System'), 'SettingsPage')
const Audit = L(() => import('./pages/admin/System'), 'Audit')

const TechHome = L(() => import('./pages/tech/Tech'), 'TechHome')
const Jobs = L(() => import('./pages/tech/Tech'), 'Jobs')
const NewJob = L(() => import('./pages/tech/Tech'), 'NewJob')
const Job = L(() => import('./pages/tech/Tech'), 'Job')
const TechStock = L(() => import('./pages/tech/Tech'), 'TechStock')
const Returns = L(() => import('./pages/tech/Tech'), 'Returns')
const TechAccount = L(() => import('./pages/tech/Tech'), 'TechAccount')

export default function App() {
  return (
    <Suspense fallback={<Loading />}>
      <Routes>
        <Route element={<ShopLayout />}>
          <Route index element={<Home />} />
          <Route path="products" element={<Products />} />
          <Route path="products/:slug" element={<ProductPage />} />
          <Route path="accessories" element={<Accessories />} />
          <Route path="cart" element={<Cart />} />
          <Route path="checkout" element={<RequireAuth><Checkout /></RequireAuth>} />
          <Route path="guarantee" element={<GuaranteePublic />} />
          <Route path="account" element={<RequireAuth perms={['customer.portal']}><AccountLayout /></RequireAuth>}>
            <Route index element={<AccountHome />} />
            <Route path="orders" element={<MyOrders />} />
            <Route path="orders/:id" element={<MyOrder />} />
            <Route path="guarantees" element={<MyGuarantees />} />
            <Route path="addresses" element={<Addresses />} />
            <Route path="wishlist" element={<Wishlist />} />
            <Route path="profile" element={<Profile />} />
          </Route>
        </Route>
        <Route path="login" element={<Login />} />
        <Route path="kiosk" element={<Kiosk />} />
        <Route path="leave" element={<LeavePublic />} />
        <Route path="technician/apply" element={<TechApply />} />
        <Route path="s/:token" element={<SurveyPublic />} />

        <Route path="admin" element={<RequireAuth perms={['reports.read', 'shop.read', 'service.read', 'wh.read', 'wh.request', 'hr.read', 'users.manage', 'catalog.manage']}><AdminLayout /></RequireAuth>}>
          <Route index element={<Overview />} />
          <Route path="orders" element={<Orders />} />
          <Route path="orders/:id" element={<OrderDetail />} />
          <Route path="products" element={<Products_ />} />
          <Route path="categories" element={<Categories />} />
          <Route path="parts" element={<Parts />} />
          <Route path="coupons" element={<Coupons />} />
          <Route path="guarantees" element={<Guarantees />} />
          <Route path="guarantees/:id" element={<GuaranteeDetail />} />
          <Route path="customers" element={<Customers />} />
          <Route path="service" element={<ServicePipeline />} />
          <Route path="repairs/:id" element={<RepairDetail />} />
          <Route path="part-requests" element={<PartRequests />} />
          <Route path="shipments" element={<Shipments />} />
          <Route path="shipments/:id" element={<ShipmentDetail />} />
          <Route path="technicians" element={<Technicians />} />
          <Route path="technicians/:id" element={<TechnicianDetail />} />
          <Route path="warehouse" element={<Stock />} />
          <Route path="warehouse/scan" element={<Scan />} />
          <Route path="warehouse/documents" element={<Documents />} />
          <Route path="warehouse/counts" element={<Counts />} />
          <Route path="warehouse/labels" element={<Labels />} />
          <Route path="hr" element={<HrDashboard />} />
          <Route path="hr/attendance" element={<Attendance />} />
          <Route path="hr/employees" element={<Employees />} />
          <Route path="hr/report" element={<HrReport />} />
          <Route path="hr/leaves" element={<Leaves />} />
          <Route path="hr/tasks" element={<Tasks />} />
          <Route path="hr/devices" element={<Devices />} />
          <Route path="hr/shifts" element={<Shifts />} />
          <Route path="surveys" element={<Surveys />} />
          <Route path="surveys/new" element={<SurveyEditor />} />
          <Route path="surveys/:id/edit" element={<SurveyEditor />} />
          <Route path="surveys/:id" element={<SurveyResults />} />
          <Route path="users" element={<Users />} />
          <Route path="roles" element={<Roles />} />
          <Route path="sms" element={<Sms />} />
          <Route path="settings" element={<SettingsPage />} />
          <Route path="audit" element={<Audit />} />
        </Route>

        <Route path="tech" element={<RequireAuth perms={['tech.portal']}><TechLayout /></RequireAuth>}>
          <Route index element={<TechHome />} />
          <Route path="jobs" element={<Jobs />} />
          <Route path="jobs/new" element={<NewJob />} />
          <Route path="jobs/:id" element={<Job />} />
          <Route path="stock" element={<TechStock />} />
          <Route path="returns" element={<Returns />} />
          <Route path="account" element={<TechAccount />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
  )
}
