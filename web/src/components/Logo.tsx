import logoUrl from '../assets/logo.png'

type Props = {
  className?: string
  alt?: string
}

export default function Logo({ className, alt = 'Bindery' }: Props) {
  return <img src={logoUrl} alt={alt} className={className} />
}
