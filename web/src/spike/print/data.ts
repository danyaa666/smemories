// Synthetic data for spike T-054: invented Vietnamese names, long text, emoji. No real people.
// A friend's note is a set of answers keyed by field id (decision D-21), plus up to two photos.
export type Friend = {
  id: number;
  name: string;
  nickname: string;
  birthday: string;
  phone: string;
  social: string;
  email: string;
  rating: number;
  answers: { how_we_met: string; first_impression: string; best_memory: string; wish: string };
  photos: string[];
};

export type Book = {
  owner: {
    name: string;
    nickname: string;
    birthday: string;
    zodiac: string;
    className: string;
    hobbies: string;
    bestFriends: string;
    food: string;
    song: string;
    movie: string;
    subject: string;
    dream: string;
    miss: string;
    motto: string;
  };
  school: string;
  city: string;
  year: number;
  quote: string;
  letter: string[];
  coverPhoto: string;
  profilePhoto: string;
  friends: Friend[];
};

const photoModules = import.meta.glob("./fixtures/photo-*.jpg", {
  eager: true,
  query: "?url",
  import: "default",
}) as Record<string, string>;
export const PHOTOS = Object.keys(photoModules)
  .sort()
  .map((k) => photoModules[k] as string);

const names = [
  ["Nguyễn Thị Hồng Ánh", "Ánh Ếch"],
  ["Trần Quốc Việt Hưng", "Hưng Béo"],
  ["Lê Phương Thảo", "Thảo Mít"],
  ["Phạm Đức Anh", "Bin"],
  ["Hoàng Thị Mỹ Duyên", "Duyên Lùn"],
  ["Võ Minh Quân", "Quân Cận"],
  ["Đặng Ngọc Bích Trâm", "Trâm Anh"],
  ["Bùi Thanh Tùng", "Tùng Sơn"],
  ["Ngô Thị Kim Ngân", "Ngân Xù"],
  ["Dương Hoàng Long", "Long Ròm"],
  ["Lý Thị Tuyết Nhung", "Nhung Nhí"],
  ["Đỗ Gia Huy", "Huy Còi"],
  ["Trịnh Khánh Linh", "Linh Lợn"],
  ["Phan Văn Trường", "Trường Gấu"],
  ["Huỳnh Ngọc Diệp", "Diệp Chi"],
  ["Mai Quang Vinh", "Vinh Bự"],
  ["Tạ Thị Thu Hiền", "Hiền Hà Nội"],
  ["Châu Nhật Minh", "Minh Mèo"],
  ["Lương Bảo Châu", "Châu Chấu"],
  ["Đinh Tiến Đạt", "Đạt Tây"],
];

const how = [
  "Mình gặp nhau ngày đầu lớp 10, cậu ngồi bàn trên và quay xuống hỏi mượn cây bút. Từ đó mượn mãi không trả 😄",
  "Hồi học thêm môn Toán của cô Hạnh, cậu giải bài hình hộ mình trên bảng và cả lớp vỗ tay.",
  "Cùng đội văn nghệ năm lớp 11: cậu hát lạc tông mà vẫn tự tin nhất đội!",
  "Trên xe buýt số 14 mỗi sáng. Mình nhận ra cậu vì chiếc cặp có con gấu bông nhỏ xíu.",
];
const imp = [
  "Cứ tưởng cậu khó gần vì hay ngồi đọc sách một mình, hóa ra cậu hài hước kinh khủng.",
  "Lúc đầu mình nghĩ cậu là học sinh gương mẫu lạnh lùng. Sai bét! 🤣",
  "Cậu ồn ào, vui tính, và luôn mang dư đồ ăn vặt cho cả bàn.",
];
const best = [
  "Chuyến dã ngoại ở Đà Lạt: lạc đường lên đồi thông, cả nhóm cười đến mức không đi nổi, rồi cùng ngắm hoàng hôn trên đỉnh đồi. Mình sẽ nhớ mãi mùi trà nóng và tiếng đàn guitar của cậu. 🌄🎸",
  "Đêm chung kết bóng đá: mình thua 0-3 nhưng cậu vẫn bắt đầu hát quốc ca và cả khán đài hát theo.",
  "Buổi chiều mưa, cả lớp trốn dưới mái hiên trường chia nhau gói bánh tráng trộn. Không có gì to tát, nhưng mình nhớ nhất.",
  "Hôm sinh nhật cậu, cả lớp giấu bánh trong ngăn bàn giáo viên. Cô chủ nhiệm cười suốt cả tiết. 🎂",
];
const wish = [
  "Chúc cậu đỗ vào ngôi trường mơ ước, luôn khỏe, luôn cười và đừng quên nhóm mình nhé! ❤️",
  "Mong cậu giữ mãi sự tử tế và ngốc nghếch dễ thương này. Hẹn gặp lại ở 20 năm sau!",
  "Chúc cậu thành công, và nếu có giàu thì nhớ mời cả lớp ăn lẩu. 🍲",
];

// One deliberately huge note: 1,496 characters (the notes form allows 2,000, T-034).
const huge = (
  "Gửi cậu, người bạn mà mình may mắn gặp trong những năm tháng đẹp nhất. " +
  "Ba năm cấp ba trôi qua nhanh như một cơn gió, nhưng những gì cậu để lại trong mình thì không gió nào thổi bay được. "
)
  .repeat(8)
  .slice(0, 1990);

export function makeBook(): Book {
  const friends: Friend[] = names.map(([name, nickname], i) => {
    return {
      id: i + 1,
      name: name as string,
      nickname: nickname as string,
      birthday: `${String(((i * 7) % 28) + 1).padStart(2, "0")}/${String(((i * 5) % 12) + 1).padStart(2, "0")}/2008`,
      phone: `09${String(10000000 + i * 3737371).slice(0, 8)}`,
      social:
        i % 2
          ? `zalo.me/${(nickname as string).toLowerCase().replace(/\s/g, "")}`
          : `@${(nickname as string).toLowerCase().replace(/\s/g, ".")}`,
      email: `ban${i + 1}@example.test`,
      rating: 3 + (i % 3),
      answers: {
        how_we_met: how[i % how.length] as string,
        first_impression: imp[i % imp.length] as string,
        best_memory: i === 7 ? huge : (best[i % best.length] as string),
        wish: i === 11 ? huge.slice(0, 400) : (wish[i % wish.length] as string),
      },
      photos: [],
    };
  });
  // 30 photos: cover, profile, then 28 over the 20 friends (8 friends get two, 12 get one).
  let p = 2;
  friends.forEach((f, i) => {
    const n = i % 5 < 2 && i < 20 ? 2 : 1;
    for (let k = 0; k < n && p < PHOTOS.length; k++) f.photos.push(PHOTOS[p++] as string);
  });
  return {
    owner: {
      name: "Nguyễn Thị Hồng Ánh",
      nickname: "Ánh Ếch",
      birthday: "14/03/2008",
      zodiac: "Song Ngư",
      className: "12A3",
      hobbies: "Vẽ, đọc truyện, chụp ảnh phim",
      bestFriends: "Thảo, Duyên, Trâm",
      food: "Bún bò Huế",
      song: "Ngày mai em đi",
      movie: "Your Name",
      subject: "Ngữ văn",
      dream:
        "Trở thành kiến trúc sư, thiết kế những thư viện nhỏ xinh ở khắp các thành phố mà mình đi qua.",
      miss: "Tiếng trống trường, ghế đá sân sau và những buổi trưa ngủ gục trên bàn học.",
      motto: "Cứ đi rồi sẽ tới ✨",
    },
    school: "THPT Lê Quý Đôn",
    city: "Đà Nẵng",
    year: 2026,
    quote: "Chúng ta rồi sẽ lớn, nhưng kỷ niệm thì ở lại.",
    letter: [
      "Thân gửi các em học sinh khóa 2023 – 2026, những người đã cùng nhau đi qua ba năm với bao nhiêu thử thách: những kỳ thi căng thẳng, những buổi sinh hoạt lớp rộn ràng và cả những lần vấp ngã.",
      "Nhà trường tự hào về từng bạn. Các em đã giữ gìn truyền thống đoàn kết, biết sẻ chia và luôn nở nụ cười, ngay cả khi con đường phía trước còn nhiều ẩn số.",
      "Xin cảm ơn quý phụ huynh, quý thầy cô và toàn thể cán bộ nhân viên đã đồng hành cùng các em. Chúc các em vững vàng bước tiếp. 🎓",
    ],
    coverPhoto: PHOTOS[0] as string,
    profilePhoto: PHOTOS[1] as string,
    friends,
  };
}

export const STRINGS = {
  en: {
    mine: "All About Me",
    friend: "From a Friend",
    touch: "Let’s Stay in Touch",
    book: "Memory Book",
    belongs: "this book belongs to",
    write: "please\nwrite\nin me!",
    howMet: "How we met",
    impression: "Your first impression of me",
    memory: "Our best memory together",
    wishFor: "Your wish for me",
    dream: "My dream for the future",
    miss: "What I’ll miss most",
    motto: "My motto",
    fullName: "Full name",
    nick: "Nickname",
    birthday: "Birthday",
    zodiac: "Zodiac sign",
    cls: "Class",
    hobbies: "Hobbies",
    best: "Best friend(s)",
    food: "Food",
    song: "Song",
    movie: "Movie",
    subject: "Subject",
    phone: "Phone",
    social: "Facebook / Zalo / IG",
    email: "Email",
    name: "Name",
    rate: "Rate our friendship",
    signed: "Signed",
    sub: "so we never lose each other",
    thanks: "thank you for the memories",
    letterTitle: "Dear Class of",
    message: "A message to the graduates",
    auto: "Autographs",
    words: "Leave a few words",
    farewell: "Farewell, Class of",
    gradYearbook: "The Graduating Class Yearbook",
  },
  vi: {
    mine: "Về mình",
    friend: "Từ một người bạn",
    touch: "Giữ liên lạc nhé",
    book: "Sổ lưu bút",
    belongs: "cuốn sổ này là của",
    write: "viết\nvào\nđây nhé!",
    howMet: "Mình quen nhau thế nào",
    impression: "Ấn tượng đầu tiên về mình",
    memory: "Kỷ niệm đẹp nhất của tụi mình",
    wishFor: "Lời chúc dành cho mình",
    dream: "Ước mơ tương lai",
    miss: "Điều mình sẽ nhớ nhất",
    motto: "Châm ngôn",
    fullName: "Họ và tên",
    nick: "Biệt danh",
    birthday: "Ngày sinh",
    zodiac: "Cung hoàng đạo",
    cls: "Lớp",
    hobbies: "Sở thích",
    best: "Bạn thân",
    food: "Món ăn",
    song: "Bài hát",
    movie: "Phim",
    subject: "Môn học",
    phone: "Điện thoại",
    social: "Facebook / Zalo / IG",
    email: "Email",
    name: "Họ tên",
    rate: "Chấm điểm tình bạn",
    signed: "Ký tên",
    sub: "để chúng mình không lạc nhau",
    thanks: "cảm ơn vì những kỷ niệm",
    letterTitle: "Gửi các bạn khóa",
    message: "Lời nhắn gửi các bạn tốt nghiệp",
    auto: "Lưu bút",
    words: "Để lại vài dòng",
    farewell: "Tạm biệt khóa",
    gradYearbook: "Kỷ yếu lớp tốt nghiệp",
  },
} as const;
export type Lang = keyof typeof STRINGS;
