import { useTranslation } from "react-i18next";
import { lazy, Suspense } from "react";
import { Outlet, Route, Routes } from "react-router-dom";
import { RequireAuth } from "./auth/RequireAuth";
import { AuthNav } from "./components/AuthNav";
import { LanguageSwitcher } from "./components/LanguageSwitcher";
import { YearbookLayout } from "./components/YearbookLayout";
import { LegalLinks } from "./components/LegalConsent";
import { LegalPage } from "./legal/LegalPage";
import { AcceptTerms } from "./pages/AcceptTerms";
import { Account } from "./pages/Account";
import { ClassAssemble } from "./pages/ClassAssemble";
import { ClassDetail } from "./pages/ClassDetail";
import { ClassNew } from "./pages/ClassNew";
import { Classes } from "./pages/Classes";
import { JoinClass } from "./pages/JoinClass";
import { ForgotPassword } from "./pages/ForgotPassword";
import { Login } from "./pages/Login";
import { Register } from "./pages/Register";
import { ResetPassword } from "./pages/ResetPassword";
import { VerifyEmail } from "./pages/VerifyEmail";
import { Home } from "./pages/Home";
import { NoteForm } from "./pages/NoteForm";
import { NotFound } from "./pages/NotFound";
import { NoteLinks } from "./pages/NoteLinks";
import { NotesInbox } from "./pages/NotesInbox";
import { YearbookEdit } from "./pages/YearbookEdit";
import { YearbookNew } from "./pages/YearbookNew";
import { Yearbooks } from "./pages/Yearbooks";

// The book screen carries its own CSS and the book fonts, so it is a separate chunk (T-058).
const Book = lazy(() => import("./pages/Book").then((m) => ({ default: m.Book })));
const Customise = lazy(() => import("./pages/Customise").then((m) => ({ default: m.Customise })));
const ClassBook = lazy(() => import("./pages/ClassBook").then((m) => ({ default: m.ClassBook })));
// Review page for the asset pack (T-151); `import.meta.env.DEV` is false in a build, so the page and its chunk are not shipped.
const ContactSheet = import.meta.env.DEV ? lazy(() => import("./book/assets/ContactSheet")) : null;

export function App() {
  return (
    <Routes>
      <Route path="/c/:token" element={<NoteForm />} />
      {ContactSheet && (
        <Route
          path="/dev/assets"
          element={
            <Suspense fallback={null}>
              <ContactSheet />
            </Suspense>
          }
        />
      )}
      <Route element={<PublicLayout />}>
        <Route path="/" element={<Home />} />
        <Route path="/login" element={<Login />} />
        <Route path="/register" element={<Register />} />
        <Route path="/forgot-password" element={<ForgotPassword />} />
        <Route path="/reset-password" element={<ResetPassword />} />
        <Route path="/terms" element={<LegalPage doc="terms" />} />
        <Route path="/join/:token" element={<JoinClass />} />
        <Route path="/privacy" element={<LegalPage doc="privacy" />} />
        <Route element={<RequireAuth />}>
          <Route path="/verify-email" element={<VerifyEmail />} />
          <Route path="/accept-terms" element={<AcceptTerms />} />
        </Route>
        <Route path="*" element={<NotFound />} />
      </Route>
      <Route element={<RequireAuth />}>
        <Route element={<YearbookLayout />}>
          <Route path="/account" element={<Account />} />
          <Route path="/classes" element={<Classes />} />
          <Route path="/classes/new" element={<ClassNew />} />
          <Route path="/classes/:id" element={<ClassDetail />} />
          <Route path="/classes/:id/assemble" element={<ClassAssemble />} />
          <Route
            path="/classes/:id/book"
            element={
              <Suspense fallback={null}>
                <ClassBook />
              </Suspense>
            }
          />
          <Route path="/yearbooks" element={<Yearbooks />} />
          <Route path="/yearbooks/new" element={<YearbookNew />} />
          <Route path="/yearbooks/:id" element={<YearbookEdit />} />
          <Route path="/yearbooks/:id/notes" element={<NotesInbox />} />
          <Route path="/yearbooks/:id/note-links" element={<NoteLinks />} />
          <Route
            path="/yearbooks/:id/book"
            element={
              <Suspense fallback={null}>
                <Book />
              </Suspense>
            }
          />
          <Route
            path="/yearbooks/:id/customise"
            element={
              <Suspense fallback={null}>
                <Customise />
              </Suspense>
            }
          />
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
      <footer className="site-footer">
        <LegalLinks />
      </footer>
    </>
  );
}
