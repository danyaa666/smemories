import type { ComponentType } from "react";
import type { BookData, BookNote } from "../../api/book";
import type { DesignIds } from "../design";
import { basic } from "./basic";
import { memoryBook } from "./memory";
import { navyClassic } from "./navy";

/**
 * A template is a set of page components registered under its id (`yearbook.template_id`). The notes page repeats:
 * `notesPerPage` is its declared capacity, so a book of N notes has ceil(N / notesPerPage) of them (0 notes: none).
 * Text that does not fit its box shrinks and is reported (fit.tsx); it never moves a note to another page.
 */
export type Template = {
  id: string;
  notesPerPage: number;
  /** Which photos the design shows (default: all). The book loads only those before it allows printing. */
  photos?: { cover: boolean; notes: boolean };
  /** The theme and font set ids it draws, when the owner can choose them (E09). */
  design?: DesignIds;
  /** Text id to the locale key of the template's own wording of it, for the texts the owner can replace. */
  textDefaults?: Record<string, string>;
  Cover: ComponentType<{ data: BookData }>;
  Profile: ComponentType<{ data: BookData }>;
  /** `first` is the index of the first note of the page in the book, `data` the whole book (title, year for the footer). */
  Notes: ComponentType<{ notes: BookNote[]; data: BookData; first: number }>;
  Back: ComponentType<{ data: BookData }>;
};

const TEMPLATES: Record<string, Template> = {
  [basic.id]: basic,
  [memoryBook.id]: memoryBook,
  [navyClassic.id]: navyClassic,
};

/** The template of that id, else the built-in one (a book whose template was never chosen, or one this build does not know). */
export const getTemplate = (id: string | null): Template =>
  (id !== null && Object.hasOwn(TEMPLATES, id) ? TEMPLATES[id] : undefined) ?? basic;

/** Whether this build can draw the template: the picker offers only those (a template the server lists but the web lacks would draw as the plain layout). */
export const knowsTemplate = (id: string): boolean => Object.hasOwn(TEMPLATES, id);
