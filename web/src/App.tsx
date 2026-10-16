import { useTranslation } from "react-i18next";
import { lazy, Suspense } from "react";
import { Outlet, Route, Routes, useLocation } from "react-router-dom";
import { RequireAuth } from "./auth/RequireAuth";
import { AuthNav } from "./components/AuthNav";
import { LanguageSwitcher } from "./components/LanguageSwitcher";
import { YearbookLayout } from "./components/YearbookLayout";
import { Account } from "./pages/Account";
import { ForgotPassword } from "./pages/ForgotPassword";
import { Login } from "./pages/Login";
import { Register } from "./pages/Register";
import { ResetPassword } from "./pages/ResetPassword";
import { VerifyEmail } from "./pages/VerifyEmail";
import { Home } from "./pages/Home";
import { NoteForm } from "./pages/NoteForm";
import { NotFound } from "./pages/NotFound";
import { YearbookEdit } from "./pages/YearbookEdit";
import { YearbookNew } from "./pages/YearbookNew";
import { Yearbooks } from "./pages/Yearbooks";

// Spike T-054 (browser print-to-PDF). Dev server only: the condition is false in a production build, so the chunk is not emitted.
const PrintSpike = import.meta.env.DEV ? lazy(() => import("./spike/print/PrintSpike")) : null;

export function App() {
  const { pathname } = useLocation();
  if (PrintSpike && pathname === "/spike/print") {
    return (
      <Suspense fallback={null}>
        <PrintSpike />
      </Suspense>
    );
  }
  return (
    <Routes>
      <Route path="/n/:token" element={<NoteForm />} />
      <Route element={<PublicLayout />}>
        <Route path="/" element={<Home />} />
        <Route path="/login" element={<Login />} />
        <Route path="/register" element={<Register />} />
        <Route path="/forgot-password" element={<ForgotPassword />} />
        <Route path="/reset-password" element={<ResetPassword />} />
        <Route element={<RequireAuth />}>
          <Route path="/verify-email" element={<VerifyEmail />} />
        </Route>
        <Route path="*" element={<NotFound />} />
      </Route>
      <Route element={<RequireAuth />}>
        <Route element={<YearbookLayout />}>
          <Route path="/account" element={<Account />} />
          <Route path="/yearbooks" element={<Yearbooks />} />
          <Route path="/yearbooks/new" element={<YearbookNew />} />
          <Route path="/yearbooks/:id" element={<YearbookEdit />} />
        </Route>
      </Route>
    </Routes>
  );
}

/** Header and page for everything outside the signed-in shell. */
function PublicLayout() {
  const { t } = useTranslation();
  return (
    <>
      <header className="app-header">
        <strong>{t("app.name")}</strong>
        <AuthNav />
        <LanguageSwitcher />
      </header>
      <main className="plain-main">
        <Outlet />
      </main>
    </>
  );
}
